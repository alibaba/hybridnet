/*
 Copyright 2021 The Hybridnet Authors.

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

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"

	"github.com/alibaba/hybridnet/pkg/daemon/iptables"
	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
	"github.com/vishvananda/netlink"
)

// Results of "ip rule" command are supposed to like this:
//
//    rule pref 0
//    |          ...(other rules)
//    |
//    |
//    |       local-pod-direct rule
//    |                v (followed with)
//    |     to-overlay-pod-subnet rule
//    |                v (followed with)
//    |         overlay-mark rule
//    |                v (followed with)
//    |     from-every-pod-subnet rules
//    |                ...
//    |     from-every-pod-subnet rules
//    |
//    |
//    |          ...(other rules)
//    v
//    rule pref 32767

// Local-pod-direct table doesn't need to be maintained manually,
// because route will be deleted by kernel while specific device is not exist.

type Manager struct {
	// Use fixed table num to mark "local-pod-direct rule"
	localDirectTableNum int

	// Use fixed table num to mark "to-overlay-pod-subnet rule"
	toOverlaySubnetTableNum int

	// Use fixed table num to mark "overlay-mark-table rule"
	overlayMarkTableNum int

	// Vxlan interface name.
	overlayIfName string

	family int

	localClusterOverlaySubnetInfoMap  SubnetInfoMap
	localClusterUnderlaySubnetInfoMap SubnetInfoMap
	localTotalSubnetInfoMap           SubnetInfoMap

	// add cluster-mesh remote subnet info
	remoteOverlaySubnetInfoMap  SubnetInfoMap
	remoteUnderlaySubnetInfoMap SubnetInfoMap
}

func CreateRouteManager(localDirectTableNum, toOverlaySubnetTableNum, overlayMarkTableNum, family int) (*Manager, error) {
	// Check if route tables are being used by others.
	if empty, err := checkIfRouteTableEmpty(localDirectTableNum, family); err != nil {
		return nil, fmt.Errorf("failed to check table %v empty: %v", localDirectTableNum, err)
	} else if !empty {
		routes, err := listRoutesByTable(localDirectTableNum, family)
		if err != nil {
			return nil, fmt.Errorf("failed to list routes for local direct table %v: %v", localDirectTableNum, err)
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
				return nil, fmt.Errorf("failed to find veth interface by index %v: %v", route.LinkIndex, err)
			}

			ones, bits := route.Dst.Mask.Size()

			// If Dst's mask is not full ones, this table is being used
			if route.Gw != nil || ones != bits || vethIf.Type() != "veth" {
				return nil, fmt.Errorf("local direct route table %v is used by others", localDirectTableNum)
			}
		}
	}

	if empty, err := checkIfRouteTableEmpty(toOverlaySubnetTableNum, family); err != nil {
		return nil, fmt.Errorf("failed to check table %v empty: %v", toOverlaySubnetTableNum, err)
	} else if !empty {
		routes, err := listRoutesByTable(toOverlaySubnetTableNum, family)
		if err != nil {
			return nil, fmt.Errorf("failed to list routes for to overlay subnet route table %v: %v", toOverlaySubnetTableNum, err)
		}

		for _, route := range routes {
			if route.LinkIndex <= 0 && !isExcludeRoute(&route) {
				return nil, fmt.Errorf("find no device route, to overlay subnet route table %v is used by others", toOverlaySubnetTableNum)
			}

			if route.LinkIndex > 0 {
				routeIf, err := netlink.LinkByIndex(route.LinkIndex)
				if err != nil {
					return nil, fmt.Errorf("failed to get route interface by index %v: %v", route.LinkIndex, err)
				}

				// "throw" v6 routes without a specified device will always be specified to "dev lo" by kernel automatically
				if routeIf.Attrs().Flags&net.FlagLoopback != 0 && isExcludeRoute(&route) {
					continue
				}

				if route.Gw != nil || routeIf.Type() != "vxlan" {
					return nil, fmt.Errorf("to overlay subnet route table %v is used by others", toOverlaySubnetTableNum)
				}
			}
		}
	}

	if empty, err := checkIfRouteTableEmpty(overlayMarkTableNum, family); err != nil {
		return nil, fmt.Errorf("failed to check table %v empty: %v", overlayMarkTableNum, err)
	} else if !empty {
		routes, err := listRoutesByTable(overlayMarkTableNum, family)
		if err != nil {
			return nil, fmt.Errorf("failed to list routes for overlay-mark table %v: %v", overlayMarkTableNum, err)
		}

		if len(routes) != 1 {
			return nil, fmt.Errorf("overlay-mark table %v is used, cause more than on route exist", overlayMarkTableNum)
		}

		overlayIf, err := netlink.LinkByIndex(routes[0].LinkIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to find overlay interface by index %v: %v", routes[0].LinkIndex, err)
		}

		if routes[0].Dst != nil || routes[0].Gw != nil || overlayIf.Type() != "vxlan" {
			return nil, fmt.Errorf("overlay-mark table %v is used by others", overlayMarkTableNum)
		}
	}

	if family != netlink.FAMILY_V6 && family != netlink.FAMILY_V4 {
		return nil, fmt.Errorf("unsupported family %v", family)
	}

	return &Manager{
		localDirectTableNum:               localDirectTableNum,
		toOverlaySubnetTableNum:           toOverlaySubnetTableNum,
		overlayMarkTableNum:               overlayMarkTableNum,
		family:                            family,
		localTotalSubnetInfoMap:           SubnetInfoMap{},
		localClusterOverlaySubnetInfoMap:  SubnetInfoMap{},
		localClusterUnderlaySubnetInfoMap: SubnetInfoMap{},
		remoteOverlaySubnetInfoMap:        SubnetInfoMap{},
		remoteUnderlaySubnetInfoMap:       SubnetInfoMap{},
	}, nil
}

func (m *Manager) ResetInfos() {
	m.localTotalSubnetInfoMap = SubnetInfoMap{}
	m.localClusterUnderlaySubnetInfoMap = SubnetInfoMap{}
	m.localClusterOverlaySubnetInfoMap = SubnetInfoMap{}
	m.remoteOverlaySubnetInfoMap = SubnetInfoMap{}
	m.remoteUnderlaySubnetInfoMap = SubnetInfoMap{}
}

func (m *Manager) AddSubnetInfo(cidr *net.IPNet, gateway, start, end net.IP, excludeIPs []net.IP,
	forwardNodeIfName string, autoNatOutgoing, isOverlay, isUnderlayOnHost bool, mode networkingv1.NetworkMode) {

	cidrString := cidr.String()

	if _, exist := m.localTotalSubnetInfoMap[cidrString]; !exist {
		m.localTotalSubnetInfoMap[cidrString] = &SubnetInfo{
			cidr:              cidr,
			forwardNodeIfName: forwardNodeIfName,
			gateway:           gateway,
			autoNatOutgoing:   autoNatOutgoing,
			includedIPRanges:  []*daemonutils.IPRange{},
			excludeIPs:        []net.IP{},
			isUnderlayOnHost:  isUnderlayOnHost,
			mode:              mode,
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
		m.localClusterOverlaySubnetInfoMap[cidrString] = subnetInfo
	} else {
		m.localClusterUnderlaySubnetInfoMap[cidrString] = subnetInfo
	}
}

func (m *Manager) AddRemoteSubnetInfo(cidr *net.IPNet, gateway, start, end net.IP, excludeIPs []net.IP, isOverlay bool) error {
	cidrString := cidr.String()

	var subnetInfo *SubnetInfo
	if isOverlay {
		if _, exists := m.remoteOverlaySubnetInfoMap[cidrString]; !exists {
			m.remoteOverlaySubnetInfoMap[cidrString] = &SubnetInfo{
				cidr:             cidr,
				gateway:          gateway,
				includedIPRanges: []*daemonutils.IPRange{},
				excludeIPs:       []net.IP{},
			}
		}

		subnetInfo = m.remoteOverlaySubnetInfoMap[cidrString]
	} else {
		if _, exists := m.remoteUnderlaySubnetInfoMap[cidrString]; !exists {
			m.remoteUnderlaySubnetInfoMap[cidrString] = &SubnetInfo{
				cidr:             cidr,
				gateway:          gateway,
				includedIPRanges: []*daemonutils.IPRange{},
				excludeIPs:       []net.IP{},
			}
		}

		subnetInfo = m.remoteUnderlaySubnetInfoMap[cidrString]
	}

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

	return nil
}

func (m *Manager) SyncRoutes() error {
	// Ensure basic rules.
	if err := appendHighestUnusedPriorityRuleIfNotExist(nil, m.localDirectTableNum, m.family, 0, 0); err != nil {
		return fmt.Errorf("failed to append local-pod-direct rule: %v", err)
	}

	if err := appendHighestUnusedPriorityRuleIfNotExist(nil, m.toOverlaySubnetTableNum, m.family, 0, 0); err != nil {
		return fmt.Errorf("failed to append to-overlay-pod-subnet rule: %v", err)
	}

	if err := appendHighestUnusedPriorityRuleIfNotExist(nil, m.overlayMarkTableNum, m.family,
		iptables.PodToNodeBackTrafficMark, iptables.PodToNodeBackTrafficMark); err != nil {
		return fmt.Errorf("failed to append overlay-mark rule: %v", err)
	}

	// Find excluded ip ranges.
	// TODO: if CIDRs are different but overlapped, exclude IP blocks might be conflicted
	localUnderlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.localClusterUnderlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("failed to find exclude ip blocks for underlay subnet: %v", err)
	}
	remoteUnderlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.remoteUnderlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("failed to find exclude ip blocks for remote underlay subnet: %v", err)
	}

	localOverlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.localClusterOverlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("failed to find exclude ip blocks for overlay subnet: %v", err)
	}
	remoteOverlayExcludeIPBlockMap, err := findExcludeIPBlockMap(m.remoteOverlaySubnetInfoMap)
	if err != nil {
		return fmt.Errorf("failed to find exclude ip blocks for overlay subnet: %v", err)
	}

	// Sync to-overlay-pod-subnet routes
	if err := m.ensureToOverlaySubnetRoutes(combineNetMap(localOverlayExcludeIPBlockMap, remoteOverlayExcludeIPBlockMap)); err != nil {
		return fmt.Errorf("failed to ensure to-overlay-pod-subnet routes: %v", err)
	}

	// Ensure overlay-mark table rule if overlay interface exist.
	if err := m.ensureOverlayMarkRoutes(); err != nil {
		return fmt.Errorf("failed to ensure overlay-mark routes: %v", err)
	}

	ruleList, err := netlink.RuleList(m.family)
	if err != nil {
		return fmt.Errorf("failed to list rule: %v", err)
	}

	// Sync from every pod subnet rules.
	for _, rule := range ruleList {
		isFromPodSubnetRule, err := checkIsFromPodSubnetRule(rule, m.family)
		if err != nil {
			return fmt.Errorf("failed to check if rule %v is from pod subnet rule: %v", rule.String(), err)
		}

		if isFromPodSubnetRule {
			// Delete subnet rules which are not supposed to exist.
			if _, exist := m.localTotalSubnetInfoMap[rule.Src.String()]; !exist {
				rule.Family = m.family
				if err := netlink.RuleDel(&rule); err != nil {
					return fmt.Errorf("del subnet policy rule error: %v", err)
				}

				if err := clearRouteTable(rule.Table, m.family); err != nil {
					return fmt.Errorf("failed to clear route table %v: %v", rule.Table, err)
				}
			}
		}
	}

	for _, info := range m.localClusterOverlaySubnetInfoMap {
		// Append overlay from pod subnet rules which don't exist and adapt to subnet configuration
		if err := ensureFromPodSubnetRuleAndRoutes(info.forwardNodeIfName, info.cidr, info.gateway, info.autoNatOutgoing, m.family,
			combineSubnetInfoMap(m.localClusterUnderlaySubnetInfoMap, m.remoteUnderlaySubnetInfoMap),
			combineNetMap(localUnderlayExcludeIPBlockMap, remoteUnderlayExcludeIPBlockMap),
			info.mode,
		); err != nil {
			return fmt.Errorf("failed to add overlay subnet %v rule and routes: %v", info.cidr, err)
		}
	}

	for _, info := range m.localClusterUnderlaySubnetInfoMap {
		// do not need create from-pod-subnet rules for underlay subnet which is not on this host
		if !info.isUnderlayOnHost {
			continue
		}

		// Append underlay from-pod-subnet rules which don't exist and adapt to subnet configuration
		if err := ensureFromPodSubnetRuleAndRoutes(info.forwardNodeIfName, info.cidr,
			info.gateway, info.autoNatOutgoing, m.family, nil, nil, info.mode,
		); err != nil {
			return fmt.Errorf("failed to add underlay subnet %v rule and routes: %v", info.cidr, err)
		}
	}

	return nil
}

func (m *Manager) ensureToOverlaySubnetRoutes(excludeIPBlockMap map[string]*net.IPNet) error {
	// Sync to-overlay-pod-subnet routes
	toOverlaySubnetRoutes, err := listRoutesByTable(m.toOverlaySubnetTableNum, m.family)
	if err != nil {
		return fmt.Errorf("failed to list to-overlay-pod-subnet routes for table %v: %v", m.toOverlaySubnetTableNum, err)
	}

	existOverlaySubnetRouteMap := map[string]bool{}
	existRemoteOverlaySubnetRouteMap := map[string]bool{}

	for _, route := range toOverlaySubnetRoutes {
		// skip exclude routes
		if isExcludeRoute(&route) {
			continue
		}

		if _, exist := m.localClusterOverlaySubnetInfoMap[route.Dst.String()]; exist {
			existOverlaySubnetRouteMap[route.Dst.String()] = true
		} else if _, exist := m.remoteOverlaySubnetInfoMap[route.Dst.String()]; exist {
			existRemoteOverlaySubnetRouteMap[route.Dst.String()] = true
		} else if err := netlink.RouteDel(&route); err != nil {
			return fmt.Errorf("failed to delete route %v: %v", route.String(), err)
		}
	}

	for _, info := range m.localClusterOverlaySubnetInfoMap {
		if _, exist := existOverlaySubnetRouteMap[info.cidr.String()]; !exist {
			overlayLink, err := netlink.LinkByName(info.forwardNodeIfName)
			if err != nil {
				return fmt.Errorf("failed to get overlay link %v: %v", info.forwardNodeIfName, err)
			}

			if err := netlink.RouteReplace(&netlink.Route{
				Dst:       info.cidr,
				LinkIndex: overlayLink.Attrs().Index,
				Table:     m.toOverlaySubnetTableNum,
				Scope:     netlink.SCOPE_UNIVERSE,
			}); err != nil {
				return fmt.Errorf("failed to add to-overlay-pod-subnet route for %v: %v", info.cidr.String(), err)
			}
		}
	}

	// add route for remote overlay subnets
	for _, info := range m.remoteOverlaySubnetInfoMap {
		if _, exist := existRemoteOverlaySubnetRouteMap[info.cidr.String()]; !exist {
			overlayLink, err := netlink.LinkByName(m.overlayIfName)
			if err != nil {
				return fmt.Errorf("failed to get overlay link %v: %v", m.overlayIfName, err)
			}

			if err := netlink.RouteReplace(&netlink.Route{
				Dst:       info.cidr,
				LinkIndex: overlayLink.Attrs().Index,
				Table:     m.toOverlaySubnetTableNum,
				Scope:     netlink.SCOPE_UNIVERSE,
			}); err != nil {
				return fmt.Errorf("failed to add to remote overlay pod subnet route for %v: %v", info.cidr.String(), err)
			}
		}
	}

	// For the traffic of accessing overlay excluded ip addresses, should not be forced to pass through vxlan device.
	if err := ensureExcludedIPBlockRoutes(excludeIPBlockMap, m.toOverlaySubnetTableNum, m.family); err != nil {
		return fmt.Errorf("failed to ensure exclude ip block routes: %v", err)
	}
	return nil
}

func (m *Manager) ensureOverlayMarkRoutes() error {
	if m.overlayIfName != "" {
		overlayLink, err := netlink.LinkByName(m.overlayIfName)
		if err != nil {
			return fmt.Errorf("failed to get overlay link %v: %v", m.overlayIfName, err)
		}

		if err := netlink.RouteReplace(&netlink.Route{
			Dst:       defaultRouteDstByFamily(m.family),
			LinkIndex: overlayLink.Attrs().Index,
			Table:     m.overlayMarkTableNum,
			Scope:     netlink.SCOPE_UNIVERSE,
		}); err != nil {
			return fmt.Errorf("failed to add overlay-mark: %v", err)
		}
	}

	return nil
}
