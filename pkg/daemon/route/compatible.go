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
	"strings"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/vishvananda/netlink"
)

func checkIsOldFromPodSubnetRule(rule netlink.Rule, family int) (bool, error) {
	if rule.IifName != "" || rule.OifName != "" || rule.Dst != nil || rule.Src == nil ||
		rule.Table < MinRouteTableNum || rule.Table >= MaxRouteTableNum {
		return false, nil
	}

	routes, err := listRoutesByTable(rule.Table, family)
	if err != nil {
		return false, fmt.Errorf("failed to list route for table %v: %v", rule.Table, err)
	}

	for _, route := range routes {
		// skip exclude routes
		if isExcludeRoute(&route) {
			continue
		}

		link, err := netlink.LinkByIndex(route.LinkIndex)
		if err != nil {
			return false, fmt.Errorf("failed to get link for route %v: %v", route.String(), err)
		}

		// Underlay subnet route table found.
		// For a vlan route table, only one default route and one subnet route will exist.
		// For a bgp route table, only one default route will exist.
		if route.Dst == nil && len(routes) == 2 || len(routes) == 1 {
			return true, nil
		}

		// overlay subnet route table found
		if strings.Contains(link.Attrs().Name, constants.VxlanLinkInfix) &&
			!(route.Dst != nil && !route.Dst.IP.IsGlobalUnicast()) {
			return true, nil
		}

	}

	return false, nil
}

func updateOldFromPodSubnetRuleToNew(rule netlink.Rule) error {
	newRule := netlink.NewRule()

	newRule.Src = rule.Src
	newRule.Priority = rule.Priority
	newRule.Table = rule.Table
	newRule.Mark = fromRuleMark
	newRule.Mask = fromRuleMask

	if err := netlink.RuleAdd(newRule); err != nil {
		return fmt.Errorf("failed to add new rule %v: %v", newRule.String(), err)
	}

	if err := netlink.RuleDel(&rule); err != nil {
		return fmt.Errorf("failed to delete old rule %v: %v", rule.String(), err)
	}

	return nil
}
