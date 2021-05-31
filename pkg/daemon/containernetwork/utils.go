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

package containernetwork

import (
	"errors"
	"fmt"
	"net"
	"strings"

	daemonutils "github.com/oecp/rama/pkg/daemon/utils"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"
)

type IPInfo struct {
	Addr net.IP
	Gw   net.IP
	Cidr *net.IPNet
}

func GenerateVlanNetIfName(parentName string, vlanId *uint32) (string, error) {
	if vlanId == nil {
		return "", fmt.Errorf("vlan id should not be nil")
	}

	if *vlanId > 4096 {
		return "", fmt.Errorf("vlan id's value range is from 0 to 4094")
	}

	if *vlanId == 0 {
		return parentName, nil
	}

	return fmt.Sprintf("%s.%v", parentName, *vlanId), nil
}

func GenerateVxlanNetIfName(parentName string, vxlanId *uint32) (string, error) {
	if vxlanId == nil || *vxlanId == 0 {
		return "", fmt.Errorf("vxlan id should not be nil or zero")
	}

	maxVxlanId := uint32(1<<24 - 1)
	if *vxlanId > maxVxlanId {
		return "", fmt.Errorf("vxlan id's value range is from 1 to %d", maxVxlanId)
	}

	return fmt.Sprintf("%s%s%v", parentName, VxlanLinkInfix, *vxlanId), nil
}

func EnsureVlanIf(nodeIfName string, vlanId *uint32) (string, error) {
	nodeIf, err := netlink.LinkByName(nodeIfName)
	if err != nil {
		return "", err
	}

	vlanIfName, err := GenerateVlanNetIfName(nodeIfName, vlanId)
	if err != nil {
		return "", fmt.Errorf("failed to ensure bridge: %v", err)
	}

	// find the vlan interface to attach to bridge, create if not exist
	var vlanIf netlink.Link
	if vlanIf, err = netlink.LinkByName(vlanIfName); err != nil {
		if vlanIfName == nodeIfName {
			// Pod in the same vlan with node.
			return vlanIfName, nil
		}

		vif := &netlink.Vlan{
			VlanId:    int(*vlanId),
			LinkAttrs: netlink.NewLinkAttrs(),
		}
		vif.ParentIndex = nodeIf.Attrs().Index
		vif.Name = vlanIfName

		err = netlink.LinkAdd(vlanIf)
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

		klog.Warningf("failed to get interface %v: %v", iF, err)
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
		return nil, fmt.Errorf("list ipv4 address for link %v failed %v", link.Attrs().Name, err)
	}

	ipv6AddrList, err := netlink.AddrList(link, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("list ipv6 address for link %v failed %v", link.Attrs().Name, err)
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
		return nil, fmt.Errorf("list link failed: %v", err)
	}

	for _, link := range linkList {
		if link.Attrs().Name != exceptLinkName &&
			!strings.HasSuffix(link.Attrs().Name, ContainerHostLinkSuffix) &&
			!strings.HasPrefix(link.Attrs().Name, "docker") &&
			!strings.HasPrefix(link.Attrs().Name, "veth") {

			linkAddrList, err := ListAllAddress(link)
			if err != nil {
				return nil, fmt.Errorf("link addr for link %v failed: %v", link.Attrs().Name, err)
			}

			addrList = append(addrList, linkAddrList...)
		}
	}

	return addrList, nil
}

func CheckIPIsGlobalUnicast(ip net.IP) bool {
	return !ip.IsInterfaceLocalMulticast() && ip.IsGlobalUnicast()
}
