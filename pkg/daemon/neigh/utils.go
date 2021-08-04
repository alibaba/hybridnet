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

package neigh

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// If neigh entry not exist, return nil
func ClearStaleNeighEntryByIP(linkIndex int, ip net.IP) error {
	family := netlink.FAMILY_V4
	if ip.To4() == nil {
		family = netlink.FAMILY_V6
	}

	neighList, err := netlink.NeighList(linkIndex, family)
	if err != nil {
		return fmt.Errorf("list neigh for link index %v error: %v", linkIndex, err)
	}

	for _, neigh := range neighList {
		if neigh.IP.Equal(ip) && neigh.State == netlink.NUD_STALE {
			if err := netlink.NeighDel(&neigh); err != nil {
				return fmt.Errorf("del neigh cache %v error: %v", neigh.String(), err)
			}
		}
	}

	return nil
}

func ClearStaleNeighEntries(linkIndex int) error {
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		neighList, err := netlink.NeighList(linkIndex, family)
		if err != nil {
			return fmt.Errorf("list neigh for link index %v error: %v", linkIndex, err)
		}

		for _, neigh := range neighList {
			if neigh.State == netlink.NUD_STALE {
				if err := netlink.NeighDel(&neigh); err != nil {
					return fmt.Errorf("del neigh cache %v error: %v", neigh.String(), err)
				}
			}
		}
	}

	return nil
}
