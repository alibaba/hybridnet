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

	"github.com/gogf/gf/container/gset"
	"github.com/oecp/rama/pkg/daemon/iptables"
	daemonutils "github.com/oecp/rama/pkg/daemon/utils"
	"github.com/vishvananda/netlink"
)

// Results of "ip rule" command are supposed to like this:
//
//    rule pref 0
//    |          ...(other rules)
//    |
//    |
//    |       local pod direct rule
//    |                v (followed with)
//    |     to overlay pod subnet rule
//    |                v (followed with)
//    |       overlay mark route rule
//    |                v (followed with)
//    |     from every pod subnet rules
//    |                ...
//    |     from every pod subnet rules
//    |
//    |
//    |          ...(other rules)
//    v
//    rule pref 32767

// Local pod direct table doesn't need to be maintained manually,
// because route will be deleted by kernel while specific device is not exist.

type Manager struct {
	// Use fixed table num to mark "local pod direct rule"
	localDirectTableNum int

	// Use fixed table num to mark "to overlay pod subnet rule"
	toOverlaySubnetTableNum int

	// Use fixed table num to mark "overlay mark table rule"
	overlayMarkTableNum int

	// Vxlan interface name.
	overlayIfName string

	family int

	localOverlaySubnetInfoMap  SubnetInfoMap
	localUnderlaySubnetInfoMap SubnetInfoMap
	localTotalSubnetInfoMap    SubnetInfoMap
	localCidr                  *gset.StrSet

	// add cluster-mesh remote subnet info
	remoteOverlaySubnetInfoMap  SubnetInfoMap
	remoteUnderlaySubnetInfoMap SubnetInfoMap
	remoteSubnetTracker         *daemonutils.SubnetCidrTracker
	remoteCidr                  *gset.StrSet
}

func CreateRouteManager(localDirectTableNum, toOverlaySubnetTableNum, overlayMarkTableNum, family int) (*Manager, error) {
	// Check if route tables are being used by others.
	if empty, err := checkIfRouteTableEmpty(localDirectTableNum, family); err != nil {
		return nil, fmt.Errorf("check table %v empty failed: %v", localDirectTableNum, err)
	} else if !empty {
		routes, err := listRoutesByTable(localDirectTableNum, family)
		if err != nil {
			return nil, fmt.Errorf("list routes for local direct table %v failed: %v", localDirectTableNum, err)
		}

		for _, route := range routes {
			if route.Dst == nil {
				return nil, fmt.Errorf("local direct route table %v is used by others, and nil Dst found", localDirectTableNum)
			}

			if route.LinkIndex <= 0 {
				return nil, fmt.Errorf("find no device route, local direct route table %v is used by others", localDirectTableNum)
			}

			vethIf, err := netlink.LinkByIndex(route.LinkIndex)
			if err != nil {
				return nil, fmt.Errorf("find veth interface by index %v failed: %v", route.LinkIndex, err)
			}

			ones, bits := route.Dst.Mask.Size()

			// If Dst's mask is not full ones, this table is being used
			if route.Gw != nil || ones != bits || vethIf.Type() != "veth" {
				return nil, fmt.Errorf("local direct route table %v is used by others", localDirectTableNum)
			}
		}
	}

	if empty, err := checkIfRouteTableEmpty(toOverlaySubnetTableNum, family); err != nil {
		return nil, fmt.Errorf("check table %v empty failed: %v", toOverlaySubnetTableNum, err)
	} else if !empty {
		routes, err := listRoutesByTable(toOverlaySubnetTableNum, family)
		if err != nil {
			return nil, fmt.Errorf("list routes for to overlay subnet route table %v failed: %v", toOverlaySubnetTableNum, err)
		}

		for _, route := range routes {
			if route.LinkIndex <= 0 && !isExcludeRoute(&route) {
				return nil, fmt.Errorf("find no device route, to overlay subnet route table %v is used by others", toOverlaySubnetTableNum)
			}

			if route.LinkIndex > 0 {
				overlayIf, err := netlink.LinkByIndex(route.LinkIndex)
				if err != nil {
					return nil, fmt.Errorf("find overlay interface by index %v failed: %v", route.LinkIndex, err)
				}

				if route.Gw != nil || overlayIf.Type() != "vxlan" {
					return nil, fmt.Errorf("to overlay subnet route table %v is used by others", toOverlaySubnetTableNum)
				}
			}
		}
	}

	if empty, err := checkIfRouteTableEmpty(overlayMarkTableNum, family); err != nil {
		return nil, fmt.Errorf("check table %v empty failed: %v", overlayMarkTableNum, err)
	} else if !empty {
		routes, err := listRoutesByTable(overlayMarkTableNum, family)
		if err != nil {
			return nil, fmt.Errorf("list routes for overlay mark route table %v failed: %v", overlayMarkTableNum, err)
		}

		if len(routes) != 1 {
			return nil, fmt.Errorf("overlay mark route table %v is used, cause more than on route exist", overlayMarkTableNum)
		}

		overlayIf, err := netlink.LinkByIndex(routes[0].LinkIndex)
		if err != nil {
			return nil, fmt.Errorf("find overlay interface by index %v failed: %v", routes[0].LinkIndex, err)
		}

		if routes[0].Dst != nil || routes[0].Gw != nil || overlayIf.Type() != "vxlan" {
			return nil, fmt.Errorf("overlay mark route table %v is used by others", overlayMarkTableNum)
		}
	}

	if family != netlink.FAMILY_V6 && family != netlink.FAMILY_V4 {
		return nil, fmt.Errorf("unsupported family %v", family)
	}

	return &Manager{
		localDirectTableNum:         localDirectTableNum,
		toOverlaySubnetTableNum:     toOverlaySubnetTableNum,
		overlayMarkTableNum:         overlayMarkTableNum,
		family:                      family,
		localTotalSubnetInfoMap:     SubnetInfoMap{},
		localOverlaySubnetInfoMap:   SubnetInfoMap{},
		localUnderlaySubnetInfoMap:  SubnetInfoMap{},
		localCidr:                   gset.NewStrSet(),
		remoteOverlaySubnetInfoMap:  SubnetInfoMap{},
		remoteUnderlaySubnetInfoMap: SubnetInfoMap{},
		remoteSubnetTracker:         daemonutils.NewSubnetCidrTracker(),
		remoteCidr:                  gset.NewStrSet(),
	}, nil
}

func (m *Manager) ResetInfos() {
	m.localTotalSubnetInfoMap = SubnetInfoMap{}
	m.localUnderlaySubnetInfoMap = SubnetInfoMap{}
	m.localOverlaySubnetInfoMap = SubnetInfoMap{}
	m.localCidr.Clear()
}

func (m *Manager) AddSubnetInfo(cidr *net.IPNet, gateway, start, end net.IP, excludeIPs []net.IP,
	forwardNodeIfName string, autoNatOutgoing, isOverlay bool) {

	cidrString := cidr.String()

	if _, exist := m.localTotalSubnetInfoMap[cidrString]; !exist {
		m.localTotalSubnetInfoMap[cidrString] = &SubnetInfo{
			cidr:              cidr,
			forwardNodeIfName: forwardNodeIfName,
			gateway:           gateway,
			autoNatOutgoing:   autoNatOutgoing,
			includedIPRanges:  []*daemonutils.IPRange{},
			excludeIPs:        []net.IP{},
		}
	}

	subnetInfo := m.localTotalSubnetInfoMap[cidrString]

	if len(excludeIPs) != 0 {
		subnetInfo.excludeIPs = append(subnetInfo.excludeIPs, excludeIPs...)
	}

	if start != nil || end != nil {
		if start == nil {
			start = cidr.IP
		}

		if end == nil {
			end = daemonutils.LastIP(cidr)
		}

		if ipRange, _ := daemonutils.CreateIPRange(start, end); ipRange != nil {
			subnetInfo.includedIPRanges = append(subnetInfo.includedIPRanges, ipRange)
		}
	}

	if isOverlay {
		// overlay interface should always be the same one
		m.overlayIfName = forwardNodeIfName
		m.localOverlaySubnetInfoMap[cidrString] = subnetInfo
	} else {
		m.localUnderlaySubnetInfoMap[cidrString] = subnetInfo
	}

	m.localCidr.Add(cidrString)
}

func (m *Manager) SyncRoutes() error {
	// check out remote subnet configurations
	_, rcErr := m.configureRemote()
	if rcErr != nil {
		return fmt.Errorf("route manager detects illegal remote subnet config: %v", rcErr)
	}

	// Ensure basic rules.
	if err := appendHighestUnusedPriorityRuleIfNotExist(nil, m.localDirectTableNum, m.family, 0, 0); err != nil {
		return fmt.Errorf("append local pod direct rule failed: %v", err)
	}

	if err := appendHighestUnusedPriorityRuleIfNotExist(nil, m.toOverlaySubnetTableNum, m.family, 0, 0); err != nil {
		return fmt.Errorf("append to overlay pod subnet rule failed: %v", err)
	}

	if err := appendHighestUnusedPriorityRuleIfNotExist(nil, m.overlayMarkTableNum, m.family,
		iptables.PodToNodeBackTrafficMark, iptables.PodToNodeBackTrafficMark); err != nil {
		return fmt.Errorf("append overlay mark route rule failed: %v", err)
	}

	// Find excluded ip ranges.
	localUnderlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.localUnderlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("find exclude ip blocks for underlay subnet failed: %v", err)
	}
	remoteUnderlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.remoteUnderlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("find exclude ip blocks for remote underlay subnet failed: %v", err)
	}

	localOverlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.localOverlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("find exclude ip blocks for overlay subnet failed: %v", err)
	}
	remoteOverlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.remoteOverlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("find exclude ip blocks for overlay subnet failed: %v", err)
	}

	// Sync to overlay pod subnet routes
	if err := m.ensureToOverlaySubnetRoutes(localOverlayExcludeIPBlockMap, remoteOverlayExcludeIPBlockMap); err != nil {
		return fmt.Errorf("ensure to overlay pod subnet routes failed: %v", err)
	}

	// Ensure overlay mark table rule if overlay interface exist.
	if err := m.ensureOverlayMarkRoutes(); err != nil {
		return fmt.Errorf("ensure overlay mark routes failed: %v", err)
	}

	ruleList, err := netlink.RuleList(m.family)
	if err != nil {
		return fmt.Errorf("list rule failed: %v", err)
	}

	// Sync from every pod subnet rules.
	for _, rule := range ruleList {
		isFromPodSubnetRule, err := checkIsFromPodSubnetRule(rule, m.family)
		if err != nil {
			return fmt.Errorf("check if rule %v is from pod subnet rule failed: %v", rule.String(), err)
		}

		if isFromPodSubnetRule {
			// Delete subnet rules which are not supposed to exist.
			if _, exist := m.localTotalSubnetInfoMap[rule.Src.String()]; !exist {
				rule.Family = m.family
				if err := netlink.RuleDel(&rule); err != nil {
					return fmt.Errorf("del subnet policy rule error: %v", err)
				}

				if err := clearRouteTable(rule.Table, m.family); err != nil {
					return fmt.Errorf("clear route table %v failed: %v", rule.Table, err)
				}
			}
		}
	}

	var underlaySubnetInfoMap SubnetInfoMap

	if len(m.remoteUnderlaySubnetInfoMap) == 0 {
		// ignore remote underlay subnets
		underlaySubnetInfoMap = m.localUnderlaySubnetInfoMap
	} else {
		// consider both local and remote underlay subnets
		underlaySubnetInfoMap = make(map[string]*SubnetInfo, len(m.remoteUnderlaySubnetInfoMap)+len(m.localUnderlaySubnetInfoMap))
		for cidr, info := range m.localUnderlaySubnetInfoMap {
			underlaySubnetInfoMap[cidr] = info
		}
		for cidr, info := range m.remoteUnderlaySubnetInfoMap {
			underlaySubnetInfoMap[cidr] = info
		}
	}

	for _, info := range m.localOverlaySubnetInfoMap {
		// Append overlay from pod subnet rules which don't exist and adapter subnet configuration
		if err := ensureFromPodSubnetRuleAndRoutes(info.forwardNodeIfName, info.cidr,
			info.gateway, info.autoNatOutgoing, true, m.family,
			underlaySubnetInfoMap, localUnderlayExcludeIPBlockMap, remoteUnderlayExcludeIPBlockMap); err != nil {
			return fmt.Errorf("add subnet %v rule and routes failed: %v", info.cidr, err)
		}
	}

	for _, info := range m.localUnderlaySubnetInfoMap {
		// Append underlay from pod subnet rules which don't exist and adapter subnet configuration
		if err := ensureFromPodSubnetRuleAndRoutes(info.forwardNodeIfName, info.cidr,
			info.gateway, info.autoNatOutgoing, false, m.family,
			nil, nil, nil); err != nil {
			return fmt.Errorf("add subnet %v rule and routes failed: %v", info.cidr, err)
		}
	}

	return nil
}

func (m *Manager) ensureToOverlaySubnetRoutes(localExcludeIPBlockMap, remoteExcludeIPBlockMap map[string]*net.IPNet) error {
	// Sync to overlay pod subnet routes
	toOverlaySubnetRoutes, err := listRoutesByTable(m.toOverlaySubnetTableNum, m.family)
	if err != nil {
		return fmt.Errorf("list to overlay pod subnet routes for table %v failed: %v", m.toOverlaySubnetTableNum, err)
	}

	existOverlaySubnetRouteMap := map[string]bool{}
	existRemoteOverlaySubnetRouteMap := map[string]bool{}

	for _, route := range toOverlaySubnetRoutes {
		// skip exclude routes
		if isExcludeRoute(&route) {
			continue
		}

		_, lExist := m.localOverlaySubnetInfoMap[route.Dst.String()]
		_, rExist := m.remoteOverlaySubnetInfoMap[route.Dst.String()]

		switch {
		case lExist:
			existOverlaySubnetRouteMap[route.Dst.String()] = true
		case rExist:
			existRemoteOverlaySubnetRouteMap[route.Dst.String()] = true
		default:
			if err := netlink.RouteDel(&route); err != nil {
				return fmt.Errorf("delete route %v failed: %v", route.String(), err)
			}
		}
	}

	for _, info := range m.localOverlaySubnetInfoMap {
		if _, exist := existOverlaySubnetRouteMap[info.cidr.String()]; !exist {
			overlayLink, err := netlink.LinkByName(info.forwardNodeIfName)
			if err != nil {
				return fmt.Errorf("get overlay link %v failed: %v", info.forwardNodeIfName, err)
			}

			if err := netlink.RouteReplace(&netlink.Route{
				Dst:       info.cidr,
				LinkIndex: overlayLink.Attrs().Index,
				Table:     m.toOverlaySubnetTableNum,
				Scope:     netlink.SCOPE_UNIVERSE,
			}); err != nil {
				return fmt.Errorf("add to overlay pod subnet route for %v failed: %v", info.cidr.String(), err)
			}
		}
	}

	// add route for remote overlay subnets
	for _, info := range m.remoteOverlaySubnetInfoMap {
		if _, exist := existRemoteOverlaySubnetRouteMap[info.cidr.String()]; !exist {
			overlayLink, err := netlink.LinkByName(m.overlayIfName)
			if err != nil {
				return fmt.Errorf("get overlay link %v failed: %v", m.overlayIfName, err)
			}

			if err := netlink.RouteReplace(&netlink.Route{
				Dst:       info.cidr,
				LinkIndex: overlayLink.Attrs().Index,
				Table:     m.toOverlaySubnetTableNum,
				Scope:     netlink.SCOPE_UNIVERSE,
			}); err != nil {
				return fmt.Errorf("add to remote overlay pod subnet route for %v failed: %v", info.cidr.String(), err)
			}
		}
	}

	// For the traffic of accessing overlay excluded ip addresses, should not be forced to pass through vxlan device.
	if err := ensureExcludedIPBlockRoutes(localExcludeIPBlockMap, remoteExcludeIPBlockMap, m.toOverlaySubnetTableNum, m.family); err != nil {
		return fmt.Errorf("ensure exclude ip block routes failed: %v", err)
	}
	return nil
}

func (m *Manager) ensureOverlayMarkRoutes() error {
	if m.overlayIfName != "" {
		overlayLink, err := netlink.LinkByName(m.overlayIfName)
		if err != nil {
			return fmt.Errorf("get overlay link %v failed: %v", m.overlayIfName, err)
		}

		if err := netlink.RouteReplace(&netlink.Route{
			Dst:       defaultRouteDstByFamily(m.family),
			LinkIndex: overlayLink.Attrs().Index,
			Table:     m.overlayMarkTableNum,
			Scope:     netlink.SCOPE_UNIVERSE,
		}); err != nil {
			return fmt.Errorf("add overlay mark route failed: %v", err)
		}
	}

	return nil
}
