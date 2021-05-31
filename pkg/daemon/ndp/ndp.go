/*
Copyright 2021 The Rama Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ndp

import (
	"fmt"
	"net"
	"time"

	"github.com/mdlayher/ndp"
)

// CheckWithTimeout checks vlan network environment and duplicate ip problems,
// timeout parameter determines how long this function will exactly last.
func CheckWithTimeout(ifi *net.Interface, srcPod, gateway net.IP, timeout time.Duration) error {
	// Use link-local address as the source IPv6 address for NDP communications.
	ndpConn, srcIP, err := ndp.Dial(ifi, ndp.LinkLocal)
	if err != nil {
		return fmt.Errorf("ndp dial interface %v failed: %v", ifi.Name, err)
	}

	defer func() {
		_ = ndpConn.Close()
	}()

	if _, err := doNS(ndpConn, gateway, ifi.HardwareAddr, timeout); err != nil {
		return fmt.Errorf("ndp resolve from ip %v to gateway %v failed: %v"+
			", vlan network seems not working, please check the setting of %v's upper physical switch port first",
			srcIP.String(), gateway.String(), err, ifi.Name)
	}

	if duplicatedHw, err := doNS(ndpConn, srcPod, ifi.HardwareAddr, timeout); err == nil {
		return fmt.Errorf("pod ip %v duplicated"+
			", please check if ip %v is occupied by other machines or containers, another hw addr is %v",
			srcPod.String(), srcPod.String(), duplicatedHw.String())
	}

	if err := doGratuitous(ndpConn, srcPod, ifi.HardwareAddr); err != nil {
		return fmt.Errorf("send gratuitous ndp for pod %v failed %v", srcPod.String(), err)
	}

	return nil
}

func doNS(c *ndp.Conn, target net.IP, hwaddr net.HardwareAddr, timeout time.Duration) (net.HardwareAddr, error) {

	// Always multicast the message to the target's solicited-node multicast
	// group as if we have no knowledge of its MAC address.
	snm, err := ndp.SolicitedNodeMulticast(target)
	if err != nil {
		return nil, fmt.Errorf("failed to determine solicited-node multicast address: %v", err)
	}

	m := &ndp.NeighborSolicitation{
		TargetAddress: target,
		Options: []ndp.Option{
			&ndp.LinkLayerAddress{
				Direction: ndp.Source,
				Addr:      hwaddr,
			},
		},
	}

	if err := c.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %v", err)
	}

	if err := c.WriteTo(m, nil, snm); err != nil {
		return nil, fmt.Errorf("failed to write message: %v", err)
	}

	// Loop and wait for replies
	for {
		msg, _, _, err := c.ReadFrom()
		if err != nil {
			return nil, fmt.Errorf("read from ndp connection failed: %v", err)
		}

		// Expect neighbor advertisement messages with the correct target address.
		if na, ok := msg.(*ndp.NeighborAdvertisement); ok && na.TargetAddress.Equal(target) {
			// Got a target NA message
			for _, opt := range na.Options {
				// Ignore NA message from temporary address...
				// Find another hardware address.
				if lla, ok := opt.(*ndp.LinkLayerAddress); ok &&
					lla.Addr.String() != hwaddr.String() {
					return lla.Addr, nil
				}
			}
		}

		// Read a message, but it isn't the one we want.  Keep trying.
	}
}

func doGratuitous(c *ndp.Conn, ip net.IP, hwaddr net.HardwareAddr) error {
	m := &ndp.NeighborAdvertisement{
		Solicited:     false, // <Adam Jensen> I never asked for this...
		Override:      true,  // Should clients replace existing cache entries
		TargetAddress: ip,
		Options: []ndp.Option{
			&ndp.LinkLayerAddress{
				Direction: ndp.Target,
				Addr:      hwaddr,
			},
		},
	}
	return c.WriteTo(m, nil, net.IPv6linklocalallnodes)
}
