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

package route

import (
	"fmt"
	"net"
	"strings"

	"golang.org/x/sys/unix"

	daemonutils "github.com/oecp/rama/pkg/daemon/utils"

	"github.com/oecp/rama/pkg/daemon/containernetwork"

	"github.com/vishvananda/netlink"
)

const (
	MinRouteTableNum = 10000
	MaxRouteTableNum = 40000

	MaxRulePriority   = 32767
	NodeLocalTableNum = 255
)

type SubnetInfo struct {
	cidr              *net.IPNet
	gateway           net.IP
	forwardNodeIfName string
	autoNatOutgoing   bool
	excludeIPs        []net.IP
	includedIPRanges  []*daemonutils.IPRange
}

type SubnetInfoMap map[string]*SubnetInfo

func checkIfRouteTableEmpty(tableNum, family int) (bool, error) {
	routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table: tableNum,
	}, netlink.RT_FILTER_TABLE)

	if err != nil {
		return false, fmt.Errorf("list route for table %v failed: %v", tableNum, err)
	}

	if len(routeList) == 0 {
		return true, nil
	}

	return false, nil
}

func listRoutesByTable(tableNum, family int) ([]netlink.Route, error) {
	routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table: tableNum,
	}, netlink.RT_FILTER_TABLE)

	if err != nil {
		return nil, fmt.Errorf("list route for table %v failed: %v", tableNum, err)
	}

	return routeList, nil
}

// findHighestUnusedRulePriority find out the highest unused rule priority after node local rule
func findHighestUnusedRulePriority(family int) (int, error) {
	ruleList, err := netlink.RuleList(family)
	if err != nil {
		return -1, fmt.Errorf("list rules failed: %v", err)
	}

	priorityMap := map[int]bool{}
	nodeLocalRulePrio := 0
	for _, rule := range ruleList {
		if rule.Table == NodeLocalTableNum {
			nodeLocalRulePrio = realRulePriority(rule.Priority)
		}
		priorityMap[realRulePriority(rule.Priority)] = true
	}

	for priority := 0; priority <= MaxRulePriority; priority++ {
		if _, inUsed := priorityMap[priority]; !inUsed {
			// priority is not in used and lower than local rule
			if priority > nodeLocalRulePrio {
				return priority, nil
			}
		}
	}

	return -1, fmt.Errorf("cannot find unused rule priority")
}

func appendHighestUnusedPriorityRuleIfNotExist(src *net.IPNet, table, family int, mark, mask int) error {
	exist, _, err := checkIfRuleExist(src, table, family)
	if err != nil {
		return fmt.Errorf("check rule (src: %v, table: %v) exist failed: %v", src.String(), table, err)
	}

	if exist {
		// rule exist
		return nil
	}

	priority, err := findHighestUnusedRulePriority(family)
	if err != nil {
		return fmt.Errorf("find highest unused rule priority for to overlay subnet rule failed: %v", err)
	}

	rule := netlink.NewRule()
	rule.Src = src
	rule.Table = table
	rule.Priority = priority
	rule.Family = family
	rule.Mask = mask
	rule.Mark = mark

	if err := netlink.RuleAdd(rule); err != nil {
		return fmt.Errorf("add policy rule %v failed: %v", rule.String(), err)
	}

	return nil
}

// findEmptyRouteTable found the first empty route table in range MinRouteTableNum ~ MaxRouteTableNum
func findEmptyRouteTable(family int) (int, error) {
	for i := MinRouteTableNum; i < MaxRouteTableNum; i++ {
		empty, err := checkIfRouteTableEmpty(i, family)
		if err != nil {
			return 0, fmt.Errorf("check route table %v empty failed: %v", i, err)
		}

		if empty {
			return i, nil
		}
	}
	return 0, fmt.Errorf("cannot find empty route table in range %v~%v", MinRouteTableNum, MaxRouteTableNum)
}

func checkIsFromPodSubnetRule(rule netlink.Rule, family int) (bool, error) {
	if rule.IifName != "" || rule.OifName != "" || rule.Dst != nil || rule.Src == nil ||
		rule.Table < MinRouteTableNum || rule.Table >= MaxRouteTableNum {
		return false, nil
	}

	routes, err := listRoutesByTable(rule.Table, family)
	if err != nil {
		return false, fmt.Errorf("list route for table %v failed: %v", rule.Table, err)
	}

	for _, route := range routes {
		// skip exclude routes
		if isExcludeRoute(&route) {
			continue
		}

		link, err := netlink.LinkByIndex(route.LinkIndex)
		if err != nil {
			return false, fmt.Errorf("get link for route %v failed: %v", route.String(), err)
		}

		// underlay subnet route table found
		if route.Dst == nil && len(routes) == 2 {
			return true, nil
		}

		// overlay subnet route table found
		if strings.Contains(link.Attrs().Name, containernetwork.VxlanLinkInfix) &&
			!(route.Dst != nil && !route.Dst.IP.IsGlobalUnicast()) {
			return true, nil
		}

	}

	return false, nil
}

func clearRouteTable(table int, family int) error {
	defaultRouteDst := defaultRouteDstByFamily(family)

	routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table: table,
	}, netlink.RT_FILTER_TABLE)

	if err != nil {
		return fmt.Errorf("list route for table %v failed: %v", table, err)
	}

	for _, r := range routeList {
		if r.Dst == nil {
			r.Dst = defaultRouteDst
		}

		if err = netlink.RouteDel(&r); err != nil {
			return fmt.Errorf("delete route %v for table %v failed: %v", r.String(), table, err)
		}
	}
	return nil
}

func ensureFromPodSubnetRuleAndRoutes(forwardNodeIfName string, cidr *net.IPNet,
	gateway net.IP, autoNatOutgoing, isOverlay bool, family int, underlaySubnetInfoMap SubnetInfoMap,
	localUnderlayExcludeIPBlockMap, remoteUnderlayExcludeIPBlockMap map[string]*net.IPNet) error {

	var table int
	var err error

	ruleExist, existRule, err := checkIfRuleExist(cidr, -1, family)
	if err != nil {
		return fmt.Errorf("check rule (src: %v, table: %v) exist failed: %v", cidr.String(), table, err)
	}

	// Add subnet rule if not exist.
	if !ruleExist {
		table, err = findEmptyRouteTable(family)
		if err != nil {
			return fmt.Errorf("find empty route table failed: %v", err)
		}
	} else {
		table = existRule.Table
	}

	forwardLink, err := netlink.LinkByName(forwardNodeIfName)
	if err != nil {
		return fmt.Errorf("get forward link %v failed: %v", forwardNodeIfName, err)
	}

	if isOverlay {
		routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
			Table: table,
		}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return fmt.Errorf("list route for table %v failed: %v", table, err)
		}

		if !autoNatOutgoing {
			defaultRoute := &netlink.Route{
				Dst:       defaultRouteDstByFamily(family),
				LinkIndex: forwardLink.Attrs().Index,
				Table:     table,
				Scope:     netlink.SCOPE_UNIVERSE,
			}

			if err := netlink.RouteReplace(defaultRoute); err != nil {
				return fmt.Errorf("add overlay subnet %v default route %v failed: %v", cidr.String(), defaultRoute.String(), err)
			}

			for _, route := range routeList {
				// Delete extra useless routes.
				if route.Dst != nil {
					if err := netlink.RouteDel(&route); err != nil {
						return fmt.Errorf("delete overlay route %v for table %v failed: %v", route.String(), table, err)
					}
				}
			}

		} else {
			for _, route := range routeList {
				// skip exclude routes
				if isExcludeRoute(&route) {
					continue
				}

				if route.Dst != nil {
					if _, exist := underlaySubnetInfoMap[route.Dst.String()]; exist {
						continue
					}
				} else {
					route.Dst = defaultRouteDstByFamily(family)
				}

				// Delete extra useless routes.
				if err := netlink.RouteDel(&route); err != nil {
					return fmt.Errorf("delete overlay route %v for table %v failed: %v", route.String(), table, err)
				}
			}

			for _, subnet := range underlaySubnetInfoMap {
				subnetRoute := &netlink.Route{
					LinkIndex: forwardLink.Attrs().Index,
					Dst:       subnet.cidr,
					Table:     table,
					Scope:     netlink.SCOPE_UNIVERSE,
				}

				if err := netlink.RouteReplace(subnetRoute); err != nil {
					return fmt.Errorf("set overlay route %v for table %v failed: %v", subnetRoute.String(), table, err)
				}
			}

			// For overlay pod to access underlay excluded ip addresses, should not be forced to pass through vxlan device.
			if err := ensureExcludedIPBlockRoutes(localUnderlayExcludeIPBlockMap, remoteUnderlayExcludeIPBlockMap, table, family); err != nil {
				return fmt.Errorf("ensure exclude all ip block routes failed: %v", err)
			}
		}

	} else {
		localAddrList, err := netlink.AddrList(nil, family)
		if err != nil {
			return fmt.Errorf("list local addresses failed: %v", err)
		}

		isLocalSubnet := false
		for _, address := range localAddrList {
			if cidr.Contains(address.IP) {
				// Check if address is an enhanced address or used to connect a subnet.
				if address.Flags&unix.IFA_F_NOPREFIXROUTE == 0 {
					isLocalSubnet = true
					break
				}
			}
		}

		subnetDirectRoute := &netlink.Route{
			LinkIndex: forwardLink.Attrs().Index,
			Dst:       cidr,
			Table:     table,
			Scope:     netlink.SCOPE_UNIVERSE,
		}

		if isLocalSubnet {
			// Check if forward interface has default route which has the same gateway ip with this rama subnet.
			defaultRoute, err := containernetwork.GetDefaultRoute(family)
			if err != nil && err != daemonutils.NotExist {
				return fmt.Errorf("get default route failed: %v", err)
			}

			if defaultRoute != nil {
				if defaultRoute.LinkIndex == forwardLink.Attrs().Index &&
					defaultRoute.Gw != nil && !defaultRoute.Gw.Equal(gateway) {
					return fmt.Errorf("exist default route of forward interface %v has a different gateway %v with %v",
						forwardNodeIfName, defaultRoute.Gw, gateway)
				}
			}

			// Check if forward interface has subnet direct route.
			directRouteList, err := netlink.RouteListFiltered(family, &netlink.Route{
				LinkIndex: forwardLink.Attrs().Index,
				Dst:       cidr,
			}, netlink.RT_FILTER_OIF|netlink.RT_FILTER_DST)

			if err != nil {
				return fmt.Errorf("list direct route for interface %v and subnet %v failed: %v",
					forwardNodeIfName, cidr.String(), err)
			}

			if len(directRouteList) == 0 {
				return fmt.Errorf("forward interface %v should have direct route for local subnet %v",
					forwardNodeIfName, cidr.String())
			}

			subnetDirectRoute.Src = directRouteList[0].Src
		}

		defaultRoute := &netlink.Route{
			LinkIndex: forwardLink.Attrs().Index,
			Table:     table,
			Scope:     netlink.SCOPE_UNIVERSE,
			Flags:     int(netlink.FLAG_ONLINK),
			Gw:        gateway,
		}

		if err := netlink.RouteReplace(subnetDirectRoute); err != nil {
			return fmt.Errorf("add vlan subent %v direct route %v failed: %v", cidr.String(), subnetDirectRoute.String(), err)
		}

		if err := netlink.RouteReplace(defaultRoute); err != nil {
			return fmt.Errorf("add vlan subnet %v default route %v failed: %v", cidr.String(), defaultRoute.String(), err)
		}
	}

	// Add rule at the last in case error happens while failed to add any routes to table.
	if !ruleExist {
		priority, err := findHighestUnusedRulePriority(family)
		if err != nil {
			return fmt.Errorf("find highest unused rule priority failed: %v", err)
		}

		rule := netlink.NewRule()
		rule.Table = table
		rule.Priority = priority
		rule.Src = cidr
		rule.Family = family

		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("add rule %v failed: %v", rule, err)
		}
	}

	return nil
}

func realRulePriority(priority int) int {
	if priority == -1 {
		return 0
	}
	return priority
}

func checkIfRuleExist(src *net.IPNet, table, family int) (bool, *netlink.Rule, error) {
	ruleList, err := netlink.RuleList(family)
	if err != nil {
		return false, nil, fmt.Errorf("list subnet policy rules error: %v", err)
	}

	for _, rule := range ruleList {
		if src == rule.Src || (src != nil && rule.Src != nil && src.String() == rule.Src.String()) {
			if table > 0 {
				if rule.Table == table {
					// rule exist
					return true, &rule, nil
				}
			} else {
				// rule exist
				return true, &rule, nil
			}
		}
	}

	return false, nil, nil
}

func defaultRouteDstByFamily(family int) *net.IPNet {
	if family == netlink.FAMILY_V6 {
		return &net.IPNet{
			IP:   net.ParseIP("::").To16(),
			Mask: net.CIDRMask(0, 128),
		}
	}

	return &net.IPNet{
		IP:   net.ParseIP("0.0.0.0").To4(),
		Mask: net.CIDRMask(0, 32),
	}
}

func ensureExcludedIPBlockRoutes(localExcludeIPBlockMap, remoteExcludeIPBlockMap map[string]*net.IPNet, table, family int) error {
	excludedRouteList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table: table,
		Type:  unix.RTN_THROW,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_TYPE)

	if err != nil {
		return fmt.Errorf("list excluded routes failed: %v", err)
	}

	for _, route := range excludedRouteList {
		_, lExists := localExcludeIPBlockMap[route.Dst.String()]
		_, rExists := remoteExcludeIPBlockMap[route.Dst.String()]

		if !lExists && !rExists {
			if err := netlink.RouteDel(&route); err != nil {
				return fmt.Errorf("delete excluded route %v failed: %v", route, err)
			}
		}
	}

	for _, cidr := range localExcludeIPBlockMap {
		if err := netlink.RouteReplace(&netlink.Route{
			Dst:   cidr,
			Table: table,
			Type:  unix.RTN_THROW,
		}); err != nil {
			return fmt.Errorf("add excluded route for block %v failed: %v", cidr.String(), err)
		}
	}

	for _, cidr := range remoteExcludeIPBlockMap {
		if err := netlink.RouteReplace(&netlink.Route{
			Dst:   cidr,
			Table: table,
			Type:  unix.RTN_THROW,
		}); err != nil {
			return fmt.Errorf("add excluded route for block %v failed: %v", cidr.String(), err)
		}
	}

	return nil
}

func findExcludeIPBlockMap(subnetInfoMap SubnetInfoMap) (map[string]*net.IPNet, error) {
	excludeIPBlockMap := map[string]*net.IPNet{}
	for _, info := range subnetInfoMap {
		excludeIPBlocks, err := daemonutils.FindSubnetExcludeIPBlocks(info.cidr, info.includedIPRanges,
			info.gateway, info.excludeIPs)

		if err != nil {
			return nil, fmt.Errorf("find excluded ip blocks for subnet %v failed: %v", info.cidr, err)
		}

		for _, block := range excludeIPBlocks {
			excludeIPBlockMap[block.String()] = block
		}
	}
	return excludeIPBlockMap, nil
}

func isExcludeRoute(route *netlink.Route) bool {
	if route == nil {
		return false
	}
	return route.Type == unix.RTN_THROW
}
