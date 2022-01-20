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

package containernetwork

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"

	"github.com/vishvananda/netlink"
)

type IPInfo struct {
	Addr net.IP
	Gw   net.IP
	Cidr *net.IPNet
}

func GenerateVlanNetIfName(parentName string, vlanID *int32) (string, error) {
	if vlanID == nil {
		return "", fmt.Errorf("vlan id should not be nil")
	}

	if *vlanID > 4096 {
		return "", fmt.Errorf("vlan id's value range is from 0 to 4094")
	}

	if *vlanID == 0 {
		return parentName, nil
	}

	return fmt.Sprintf("%s.%v", parentName, *vlanID), nil
}

func GenerateVxlanNetIfName(parentName string, vlanID *int32) (string, error) {
	if vlanID == nil || *vlanID == 0 {
		return "", fmt.Errorf("vxlan id should not be nil or zero")
	}

	maxVxlanID := int32(1<<24 - 1)
	if *vlanID > maxVxlanID {
		return "", fmt.Errorf("vxlan id's value range is from 1 to %d", maxVxlanID)
	}

	return fmt.Sprintf("%s%s%v", parentName, VxlanLinkInfix, *vlanID), nil
}

func EnsureVlanIf(nodeIfName string, vlanID *int32) (string, error) {
	nodeIf, err := netlink.LinkByName(nodeIfName)
	if err != nil {
		return "", err
	}

	vlanIfName, err := GenerateVlanNetIfName(nodeIfName, vlanID)
	if err != nil {
		return "", fmt.Errorf("failed to ensure bridge: %v", err)
	}

	// create the vlan interface if not exist
	var vlanIf netlink.Link
	if vlanIf, err = netlink.LinkByName(vlanIfName); err != nil {
		if vlanIfName == nodeIfName {
			// Pod in the same vlan with node.
			return vlanIfName, nil
		}

		vif := &netlink.Vlan{
			VlanId:    int(*vlanID),
			LinkAttrs: netlink.NewLinkAttrs(),
		}
		vif.ParentIndex = nodeIf.Attrs().Index
		vif.Name = vlanIfName

		err = netlink.LinkAdd(vif)
		if err != nil {
			return vlanIfName, err
		}

		vlanIf, err = netlink.LinkByName(vlanIfName)
		if err != nil {
			return vlanIfName, err
		}
	}

	// setup the vlan (or node interface) if it's not UP
	if err = netlink.LinkSetUp(vlanIf); err != nil {
		return vlanIfName, err
	}

	return vlanIfName, nil
}

func GetDefaultInterface(family int) (*net.Interface, error) {
	defaultRoute, err := GetDefaultRoute(family)
	if err != nil {
		return nil, err
	}

	if defaultRoute.LinkIndex <= 0 {
		return nil, errors.New("found ipv4 default route but could not determine interface")
	}

	iface, err := net.InterfaceByIndex(defaultRoute.LinkIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface %v", err)
	}

	return iface, nil
}

func GetDefaultRoute(family int) (*netlink.Route, error) {
	routes, err := netlink.RouteList(nil, family)
	if err != nil {
		return nil, err
	}

	defaultDstString := "0.0.0.0/0"
	if family == netlink.FAMILY_V6 {
		defaultDstString = "::/0"
	}

	for _, route := range routes {
		if route.Dst == nil || route.Dst.String() == defaultDstString {
			return &route, nil
		}
	}

	return nil, daemonutils.NotExist
}

// GetInterfaceByPreferString return first valid interface by prefer string.
func GetInterfaceByPreferString(preferString string) (*net.Interface, error) {
	ifList := strings.Split(preferString, ",")
	for _, iF := range ifList {
		if iF == "" {
			continue
		}

		iif, err := net.InterfaceByName(iF)
		if err == nil {
			return iif, nil
		}
	}

	return nil, fmt.Errorf("no valid interface found by prefer string %v", preferString)
}

func GenerateIPListString(addrList []netlink.Addr) string {
	ipListString := ""
	for _, addr := range addrList {
		if ipListString == "" {
			ipListString = addr.IP.String()
			continue
		}
		ipListString = ipListString + "," + addr.IP.String()
	}

	return ipListString
}

func ListAllAddress(link netlink.Link) ([]netlink.Addr, error) {
	var addrList []netlink.Addr

	ipv4AddrList, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("failed to list ipv4 address for link %v: %v", link.Attrs().Name, err)
	}

	ipv6AddrList, err := netlink.AddrList(link, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("failed to list ipv6 address for link %v: %v", link.Attrs().Name, err)
	}

	for _, addr := range ipv4AddrList {
		if CheckIPIsGlobalUnicast(addr.IP) {
			addrList = append(addrList, addr)
		}
	}

	for _, addr := range ipv6AddrList {
		if CheckIPIsGlobalUnicast(addr.IP) {
			addrList = append(addrList, addr)
		}
	}

	return addrList, nil
}

func ListLocalAddressExceptLink(exceptLinkName string) ([]netlink.Addr, error) {
	var addrList []netlink.Addr

	linkList, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to list link: %v", err)
	}

	for _, link := range linkList {
		linkName := link.Attrs().Name
		if linkName != exceptLinkName && !CheckIfContainerNetworkLink(linkName) {

			linkAddrList, err := ListAllAddress(link)
			if err != nil {
				return nil, fmt.Errorf("failed to link addr for link %v: %v", link.Attrs().Name, err)
			}

			addrList = append(addrList, linkAddrList...)
		}
	}

	return addrList, nil
}

func CheckIPIsGlobalUnicast(ip net.IP) bool {
	return !ip.IsInterfaceLocalMulticast() && ip.IsGlobalUnicast()
}

func checkPodRuleExist(podCidr *net.IPNet, family int) (bool, error) {
	ruleList, err := netlink.RuleList(family)
	if err != nil {
		return false, fmt.Errorf("failed to list rule: %v", err)
	}

	for _, rule := range ruleList {
		if rule.Src != nil && podCidr.String() == rule.Src.String() {
			return true, nil
		}
	}

	return false, nil
}

func checkPodNeighExist(podIP net.IP, forwardNodeIfIndex int, family int) (bool, error) {
	neighList, err := netlink.NeighProxyList(forwardNodeIfIndex, family)
	if err != nil {
		return false, fmt.Errorf("failed to list neighs for forward node if index %v: %v", forwardNodeIfIndex, err)
	}

	for _, neigh := range neighList {
		if neigh.IP.Equal(podIP) {
			return true, nil
		}
	}

	return false, nil
}

func checkPodNetConfigReady(podIP net.IP, podCidr *net.IPNet, forwardNodeIfIndex int, family int) error {
	backOffBase := 100 * time.Microsecond
	retries := 4

	for i := 0; i < retries; i++ {
		time.Sleep(backOffBase)
		backOffBase = backOffBase * 2

		neighExist, err := checkPodNeighExist(podIP, forwardNodeIfIndex, family)
		if err != nil {
			return fmt.Errorf("failed to check pod ip %v neigh exist: %v", podIP, err)
		}

		ruleExist, err := checkPodRuleExist(podCidr, family)
		if err != nil {
			return fmt.Errorf("failed to check cidr %v rule exist: %v", podCidr, err)
		}

		if neighExist && ruleExist {
			break
		}

		if i == retries-1 {
			if !neighExist {
				return fmt.Errorf("proxy neigh for %v is not created, waiting for daemon to create it", podIP)
			}

			if !ruleExist {
				return fmt.Errorf("policy rule for %v is not created, waiting for daemon to create it", podCidr)
			}
		}
	}

	return nil
}
