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
	"os"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"golang.org/x/sys/unix"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"

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

	for _, route := range routes {
		if IsDefaultRoute(&route, family) {
			return &route, nil
		}
	}

	return nil, daemonutils.NotExist
}

func IsDefaultRoute(route *netlink.Route, family int) bool {
	if route == nil {
		return false
	}

	defaultDstString := "0.0.0.0/0"
	if family == netlink.FAMILY_V6 {
		defaultDstString = "::/0"
	}

	return route.Dst == nil || route.Dst.String() == defaultDstString
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

func checkPodRuleExist(podCidr *net.IPNet, family int) (bool, int, error) {
	ruleList, err := netlink.RuleList(family)
	if err != nil {
		return false, 0, fmt.Errorf("failed to list rule: %v", err)
	}

	for _, rule := range ruleList {
		if rule.Src != nil && podCidr.String() == rule.Src.String() {
			return true, rule.Table, nil
		}
	}

	return false, 0, nil
}

func checkDefaultRouteExist(table int, family int) (bool, error) {
	routeList, err := netlink.RouteListFiltered(family, &netlink.Route{
		Table: table,
	}, netlink.RT_FILTER_TABLE)

	if err != nil {
		return false, fmt.Errorf("failed to list route for table %v", table)
	}

	for _, route := range routeList {
		if IsDefaultRoute(&route, family) {
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

func checkPodNetConfigReady(podIP net.IP, podCidr *net.IPNet, forwardNodeIfIndex int, family int,
	networkMode networkingv1.NetworkMode) error {

	backOffBase := 100 * time.Microsecond
	retries := 5

	for i := 0; i < retries; i++ {
		switch networkMode {
		case networkingv1.NetworkModeVxlan, networkingv1.NetworkModeVlan:
			neighExist, err := checkPodNeighExist(podIP, forwardNodeIfIndex, family)
			if err != nil {
				return fmt.Errorf("failed to check pod ip %v neigh exist: %v", podIP, err)
			}

			ruleExist, _, err := checkPodRuleExist(podCidr, family)
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
		case networkingv1.NetworkModeBGP:
			ruleExist, table, err := checkPodRuleExist(podCidr, family)
			if err != nil {
				return fmt.Errorf("failed to check cidr %v rule and default route exist: %v", podCidr, err)
			}

			defaultRouteExist := false
			if ruleExist {
				defaultRouteExist, err = checkDefaultRouteExist(table, family)
				if err != nil {
					return fmt.Errorf("failed to check cidr %v default route exist: %v", podCidr, err)
				}

				if defaultRouteExist {
					break
				}
			}

			if i == retries-1 {
				if !ruleExist {
					return fmt.Errorf("policy rule for %v is not created, waiting for daemon to create it", podCidr)
				}

				if !defaultRouteExist {
					return fmt.Errorf("default route for %v is not created, waiting for daemon to create it", podCidr)
				}
			}
		default:
			break
		}

		time.Sleep(backOffBase)
		backOffBase = backOffBase * 2
	}

	return nil
}

// AddRoute adds a universally-scoped route to a device with onlink flag.
func AddRoute(ipn *net.IPNet, gw net.IP, dev netlink.Link) error {
	return netlink.RouteAdd(&netlink.Route{
		LinkIndex: dev.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Flags:     int(netlink.FLAG_ONLINK),
		Dst:       ipn,
		Gw:        gw,
	})
}

func ensureRpFilterConfigs(containerHostIf string) error {
	for _, key := range []string{"default", "all"} {
		sysctlPath := fmt.Sprintf(RpFilterSysctl, key)
		if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
			return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
		}
	}

	sysctlPath := fmt.Sprintf(RpFilterSysctl, containerHostIf)
	if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
		return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
	}

	existInterfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("error get exist interfaces on system: %v", err)
	}

	for _, existIf := range existInterfaces {
		if CheckIfContainerNetworkLink(existIf.Name) {
			continue
		}

		sysctlPath := fmt.Sprintf(RpFilterSysctl, existIf.Name)
		sysctlValue, err := daemonutils.GetSysctl(sysctlPath)
		if err != nil {
			return fmt.Errorf("error get: %s sysctl path: %v", sysctlPath, err)
		}
		if sysctlValue != 0 {
			if err = daemonutils.SetSysctl(sysctlPath, 0); err != nil {
				return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
			}
		}
	}

	return nil
}

func enableIPForward(family int) error {
	if family == netlink.FAMILY_V4 {
		return ip.EnableIP4Forward()
	}
	return ip.EnableIP6Forward()
}

func ensureNeighGCThresh(family int, neighGCThresh1, neighGCThresh2, neighGCThresh3 int) error {
	if family == netlink.FAMILY_V4 {
		// From kernel doc:
		// neigh/default/gc_thresh1 - INTEGER
		//     Minimum number of entries to keep.  Garbage collector will not
		//     purge entries if there are fewer than this number.
		//     Default: 128
		if err := daemonutils.SetSysctl(IPv4NeighGCThresh1, neighGCThresh1); err != nil {
			return fmt.Errorf("error set: %s sysctl path to %v, error: %v", IPv4NeighGCThresh1, neighGCThresh1, err)
		}

		// From kernel doc:
		// neigh/default/gc_thresh2 - INTEGER
		//     Threshold when garbage collector becomes more aggressive about
		//     purging entries. Entries older than 5 seconds will be cleared
		//     when over this number.
		//     Default: 512
		if err := daemonutils.SetSysctl(IPv4NeighGCThresh2, neighGCThresh2); err != nil {
			return fmt.Errorf("error set: %s sysctl path to %v, error: %v", IPv4NeighGCThresh2, neighGCThresh2, err)
		}

		// From kernel doc:
		// neigh/default/gc_thresh3 - INTEGER
		//     Maximum number of neighbor entries allowed.  Increase this
		//     when using large numbers of interfaces and when communicating
		//     with large numbers of directly-connected peers.
		//     Default: 1024
		if err := daemonutils.SetSysctl(IPv4NeighGCThresh3, neighGCThresh3); err != nil {
			return fmt.Errorf("error set: %s sysctl path to %v, error: %v", IPv4NeighGCThresh3, neighGCThresh3, err)
		}

		return nil
	}

	if err := daemonutils.SetSysctl(IPv6NeighGCThresh1, neighGCThresh1); err != nil {
		return fmt.Errorf("error set: %s sysctl path to %v, error: %v", IPv6NeighGCThresh1, neighGCThresh1, err)
	}

	if err := daemonutils.SetSysctl(IPv6NeighGCThresh2, neighGCThresh2); err != nil {
		return fmt.Errorf("error set: %s sysctl path to %v, error: %v", IPv6NeighGCThresh2, neighGCThresh2, err)
	}

	if err := daemonutils.SetSysctl(IPv6NeighGCThresh3, neighGCThresh3); err != nil {
		return fmt.Errorf("error set: %s sysctl path to %v, error: %v", IPv6NeighGCThresh3, neighGCThresh3, err)
	}

	return nil
}

func CheckIPv6GlobalDisabled() (bool, error) {
	moduleDisableVar, err := daemonutils.GetSysctl(IPv6DisableModuleParameter)
	if err != nil {
		return false, err
	}

	if moduleDisableVar == 1 {
		return true, nil
	}

	sysctlGlobalDisableVar, err := daemonutils.GetSysctl(fmt.Sprintf(IPv6DisableSysctl, "all"))
	if err != nil {
		return false, err
	}

	if sysctlGlobalDisableVar == 1 {
		return true, nil
	}

	return false, nil
}

func CheckIPv6Disabled(nicName string) (bool, error) {
	globalDisabled, err := CheckIPv6GlobalDisabled()
	if err != nil {
		return false, err
	}

	if globalDisabled {
		return true, nil
	}

	sysctlDisableVar, err := daemonutils.GetSysctl(fmt.Sprintf(IPv6DisableSysctl, nicName))
	if err != nil {
		return false, err
	}

	if sysctlDisableVar == 1 {
		return true, nil
	}

	return false, nil
}

// ConfigureIface takes the result of IPAM plugin and
// applies to the ifName interface.
func ConfigureIface(ifName string, res *current.Result) error {
	if len(res.Interfaces) == 0 {
		return fmt.Errorf("no interfaces to configure")
	}

	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to lookup %q: %v", ifName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to set %q UP: %v", ifName, err)
	}

	var v4gw, v6gw net.IP
	var hasEnabledIPv6 = false
	for _, ipc := range res.IPs {
		if ipc.Interface == nil {
			continue
		}
		intIdx := *ipc.Interface
		if intIdx < 0 || intIdx >= len(res.Interfaces) || res.Interfaces[intIdx].Name != ifName {
			// IP address is for a different interface
			return fmt.Errorf("failed to add IP addr %v to %q: invalid interface index", ipc, ifName)
		}

		// Make sure sysctl "disable_ipv6" is 0 if we are about to add
		// an IPv6 address to the interface
		if !hasEnabledIPv6 && ipc.Version == "6" {
			// Enabled IPv6 for loopback "lo" and the interface
			// being configured
			for _, iface := range [2]string{"lo", ifName} {
				ipv6SysctlValueName := fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", iface)

				// Read current sysctl value
				value, err := sysctl.Sysctl(ipv6SysctlValueName)
				if err != nil || value == "0" {
					// FIXME: log warning if unable to read sysctl value
					continue
				}

				// Write sysctl to enable IPv6
				_, err = sysctl.Sysctl(ipv6SysctlValueName, "0")
				if err != nil {
					return fmt.Errorf("failed to enable IPv6 for interface %q (%s=%s): %v", iface, ipv6SysctlValueName, value, err)
				}
			}
			hasEnabledIPv6 = true
		}

		addr := &netlink.Addr{
			IPNet: &ipc.Address,
			Label: "",
			Flags: unix.IFA_F_NOPREFIXROUTE,
		}
		if err = netlink.AddrAdd(link, addr); err != nil {
			return fmt.Errorf("failed to add IP addr %v to %q: %v", ipc, ifName, err)
		}

		gwIsV4 := ipc.Gateway.To4() != nil
		if gwIsV4 && v4gw == nil {
			v4gw = ipc.Gateway
		} else if !gwIsV4 && v6gw == nil {
			v6gw = ipc.Gateway
		}
	}

	if v6gw != nil {
		if err = ip.SettleAddresses(ifName, 10); err != nil {
			return fmt.Errorf("failed to settle address on %s: %v", ifName, err)
		}
	}

	for _, r := range res.Routes {
		routeIsV4 := r.Dst.IP.To4() != nil
		gw := r.GW
		if gw == nil {
			if routeIsV4 && v4gw != nil {
				gw = v4gw
			} else if !routeIsV4 && v6gw != nil {
				gw = v6gw
			}
		}
		if err = AddRoute(&r.Dst, gw, link); err != nil {
			// we skip over duplicate routes as we assume the first one wins
			if !os.IsExist(err) {
				return fmt.Errorf("failed to add route '%v via %v dev %v': %v", r.Dst, gw, ifName, err)
			}
		}
	}

	return nil
}
