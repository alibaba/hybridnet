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

package addr

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func checkIfEnhancedAddr(link netlink.Link, addr netlink.Addr, family int) (bool, error) {
	routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table:     unix.RT_TABLE_LOCAL,
		LinkIndex: link.Attrs().Index,
		Src:       addr.IP,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_OIF|netlink.RT_FILTER_SRC)

	if err != nil {
		return false, fmt.Errorf("list local routes for interface %v and src %v failed: %v",
			link.Attrs().Name, addr.IP.String(), err)
	}

	if len(routeList) == 0 && addr.Flags&unix.IFA_F_SECONDARY == 0 && addr.Flags&unix.IFA_F_NOPREFIXROUTE != 0 {
		return true, nil
	}

	return false, nil
}

func ensureSubnetEnhancedAddr(link netlink.Link, newEnhancedAddr, outOfDateEnhancedAddr *netlink.Addr, family int) error {
	if newEnhancedAddr == nil {
		return fmt.Errorf("new enhanced address should not be nil")
	}

	if outOfDateEnhancedAddr != nil {
		cidr1 := net.IPNet{
			IP:   outOfDateEnhancedAddr.IP.Mask(outOfDateEnhancedAddr.Mask),
			Mask: outOfDateEnhancedAddr.Mask,
		}

		cidr2 := net.IPNet{
			IP:   newEnhancedAddr.IP.Mask(newEnhancedAddr.Mask),
			Mask: newEnhancedAddr.Mask,
		}

		if cidr1.String() != cidr2.String() {
			return fmt.Errorf("out-of-date enhanced address %v need to be in the same subnet "+
				"with in new enhanced address %v", outOfDateEnhancedAddr, newEnhancedAddr)
		}
	}

	if err := netlink.AddrAdd(link, newEnhancedAddr); err != nil {
		return fmt.Errorf("add enhanced addr %v for interface %v failed: %v",
			newEnhancedAddr.IP.String(), link.Attrs().Name, err)
	}

	if outOfDateEnhancedAddr != nil {
		if err := netlink.AddrDel(link, outOfDateEnhancedAddr); err != nil {
			return fmt.Errorf("del out-of-date enhanced addr %v for interface %v failed: %v",
				outOfDateEnhancedAddr.IP.String(), link.Attrs().Name, err)
		}
	}

	// Seems like if an new address is assigned to a interface while an old address in the same subnet exists,
	// the new address will become a "secondary" address which has no local routes. Add once the old address
	// is deleted, the new address will get to a normal address and kernel will apply three new local routes for it.
	//
	// So local routes should be delete after the old out-of-date address is deleted.
	routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table:     unix.RT_TABLE_LOCAL,
		LinkIndex: link.Attrs().Index,
		Src:       newEnhancedAddr.IP,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_OIF|netlink.RT_FILTER_SRC)

	if err != nil {
		return fmt.Errorf("list local routes for interface %v and src %v failed: %v",
			link.Attrs().Name, newEnhancedAddr.IP.String(), err)
	}

	for _, route := range routeList {
		if err := netlink.RouteDel(&route); err != nil {
			return fmt.Errorf("delete local route %v failed: %v", route.String(), err)
		}
	}

	return nil
}
