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

package arp

import (
	"fmt"
	"net"
	"time"

	"github.com/mdlayher/ethernet"
)

// CheckWithTimeout checks vlan network environment and duplicate ip problems,
// timeout parameter determines how long this function will exactly last.
func CheckWithTimeout(ifi *net.Interface, srcPod, gateway net.IP, timeout time.Duration) error {
	// Resolve gateway ip for vlan check.
	if _, err := pingOverInterface(srcPod, gateway, ifi, timeout); err != nil {
		return fmt.Errorf("arp resolve from pod %v to gateway %v failed: %v"+
			", vlan network seems not working, please check the setting of %v's upper physical switch port first",
			srcPod.String(), gateway.String(), err, ifi.Name)
	}

	// Resolve src pod ip for duplicate ip check and send gratuitous arp.
	// Src ip should be 0.0.0.0 for arp probe.
	if duplicatedHw, err := pingOverInterface(net.ParseIP("0.0.0.0"), srcPod, ifi, timeout); err == nil {
		return fmt.Errorf("pod ip %v duplicated"+
			", please check if ip %v is occupied by other machines or containers, another hw addr is %v",
			srcPod.String(), srcPod.String(), duplicatedHw.String())
	}

	// Send gratuitous arp to ensure remote neigh cache flushed.
	if err := gratuitousOverInterface(srcPod, ifi); err != nil {
		return fmt.Errorf("send gratuitous arp for pod %v failed %v", srcPod.String(), err)
	}

	return nil
}

func pingOverInterface(srcIP, dstIP net.IP, iif *net.Interface, timeout time.Duration) (net.HardwareAddr, error) {
	client, err := Dial(iif, srcIP)
	if err != nil {
		return nil, fmt.Errorf("init client with ip %v interface %v failed: %v", srcIP.String(), iif.Name, err)
	}

	defer func() {
		_ = client.Close()
	}()

	if err := client.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("set arp client dead line error: %v", err)
	}

	hw, err := client.Resolve(dstIP)
	if err != nil {
		return nil, fmt.Errorf("resolve dst ip %v failed: %v", dstIP.String(), err)
	}

	return hw, nil
}

func gratuitousOverInterface(ip net.IP, iif *net.Interface) error {
	client, err := Dial(iif, ip)
	if err != nil {
		return fmt.Errorf("init client with ip %v interface %v failed: %v", ip.String(), iif.Name, err)
	}

	defer func() {
		_ = client.Close()
	}()

	for _, op := range []Operation{OperationRequest, OperationReply} {
		arp, err := NewPacket(op, client.ifi.HardwareAddr, client.ip, ethernet.Broadcast, ip)
		if err != nil {
			return fmt.Errorf("create arp packet failed: %v", err)
		}

		if err := client.WriteTo(arp, ethernet.Broadcast); err != nil {
			return fmt.Errorf("send gratuitous packet failed: %v", err)
		}
	}

	return nil
}
