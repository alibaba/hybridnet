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

package controller

import (
	"fmt"
	"net"

	"github.com/gogf/gf/container/gset"
	"github.com/vishvananda/netlink"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/daemon/iptables"
	"github.com/oecp/rama/pkg/daemon/neigh"
	"github.com/oecp/rama/pkg/daemon/route"
)

func (c *Controller) getRouterManager(ipVersion ramav1.IPVersion) *route.Manager {
	if ipVersion == ramav1.IPv6 {
		return c.routeV6Manager
	}
	return c.routeV4Manager
}

func (c *Controller) getNeighManager(ipVersion ramav1.IPVersion) *neigh.Manager {
	if ipVersion == ramav1.IPv6 {
		return c.neighV6Manager
	}
	return c.neighV4Manager
}

func (c *Controller) getIPtablesManager(ipVersion ramav1.IPVersion) *iptables.Manager {
	if ipVersion == ramav1.IPv6 {
		return c.iptablesV6Manager
	}
	return c.iptablesV4Manager
}

func (c *Controller) getIPInstanceByAddress(address net.IP) (*ramav1.IPInstance, error) {
	ipInstanceList, err := c.ipInstanceIndexer.ByIndex(ByInstanceIPIndexer, address.String())
	if err != nil {
		return nil, fmt.Errorf("get ip instance by ip %v indexer failed: %v", address.String(), err)
	}

	if len(ipInstanceList) > 1 {
		return nil, fmt.Errorf("get more than one ip instance for ip %v", address.String())
	}

	if len(ipInstanceList) == 1 {
		instance, ok := ipInstanceList[0].(*ramav1.IPInstance)
		if !ok {
			return nil, fmt.Errorf("transform obj to ipinstance failed")
		}

		return instance, nil
	}

	if len(ipInstanceList) == 0 {
		// not found
		return nil, nil
	}

	return nil, fmt.Errorf("ip instance for address %v not found", address.String())
}

func (c *Controller) getRemoteVtepByEndpointAddress(address net.IP) (*ramav1.RemoteVtep, error) {
	// try to find remote pod ip
	remoteVtepList, err := c.remoteVtepIndexer.ByIndex(ByEndpointIPListIndexer, address.String())
	if err != nil {
		return nil, fmt.Errorf("get remote vtep by ip %v indexer failed: %v", address.String(), err)
	}

	if len(remoteVtepList) > 1 {
		return nil, fmt.Errorf("get more than one remote vtep for ip %v", address.String())
	}

	if len(remoteVtepList) == 1 {
		vtep, ok := remoteVtepList[0].(*ramav1.RemoteVtep)
		if !ok {
			return nil, fmt.Errorf("transform obj to remote vtep failed")
		}

		return vtep, nil
	}

	if len(remoteVtepList) == 0 {
		// not found
		return nil, nil
	}

	return nil, fmt.Errorf("remote vtep for pod address %v not found", address.String())
}

func initErrorMessageWrapper(prefix string) func(string, ...interface{}) string {
	return func(format string, args ...interface{}) string {
		return prefix + fmt.Sprintf(format, args...)
	}
}

func parseSubnetSpecRangeMeta(addressRange *ramav1.AddressRange) (cidr *net.IPNet, gateway, start, end net.IP,
	excludeIPs, reservedIPs []net.IP, err error) {

	if addressRange == nil {
		return nil, nil, nil, nil, nil, nil,
			fmt.Errorf("cannot parse a nil range")
	}

	cidr, err = netlink.ParseIPNet(addressRange.CIDR)
	if err != nil {
		return nil, nil, nil, nil, nil, nil,
			fmt.Errorf("failed to parse subnet cidr %v error: %v", addressRange.CIDR, err)
	}

	gateway = net.ParseIP(addressRange.Gateway)
	if gateway == nil {
		return nil, nil, nil, nil, nil, nil,
			fmt.Errorf("invalid gateway ip %v", addressRange.Gateway)
	}

	if addressRange.Start != "" {
		start = net.ParseIP(addressRange.Start)
		if start == nil {
			return nil, nil, nil, nil, nil, nil,
				fmt.Errorf("invalid start ip %v", addressRange.Start)
		}
	}

	if addressRange.End != "" {
		end = net.ParseIP(addressRange.End)
		if end == nil {
			return nil, nil, nil, nil, nil, nil,
				fmt.Errorf("invalid end ip %v", addressRange.End)
		}
	}

	for _, ipString := range addressRange.ExcludeIPs {
		excludeIP := net.ParseIP(ipString)
		if excludeIP == nil {
			return nil, nil, nil, nil, nil, nil,
				fmt.Errorf("invalid exclude ip %v", ipString)
		}
		excludeIPs = append(excludeIPs, excludeIP)
	}

	for _, ipString := range addressRange.ReservedIPs {
		reservedIP := net.ParseIP(ipString)
		if reservedIP == nil {
			return nil, nil, nil, nil, nil, nil,
				fmt.Errorf("invalid reserved ip %v", ipString)
		}
		reservedIPs = append(reservedIPs, reservedIP)
	}

	return
}

func isAddressRangeEqual(a, b *ramav1.AddressRange) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if a.Version != b.Version ||
		a.CIDR != b.CIDR || a.Start != b.Start || a.End != b.End ||
		a.Gateway != b.Gateway ||
		!isIPListEqual(a.ReservedIPs, b.ReservedIPs) ||
		!isIPListEqual(a.ExcludeIPs, b.ExcludeIPs) {
		return false
	}

	return true
}

func isIPListEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) == 0 || len(b) == 0 {
		return false
	}

	return gset.NewStrSetFrom(a).Equal(gset.NewStrSetFrom(b))
}
