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
	"fmt"
	"net"
	"time"

	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/oecp/rama/pkg/daemon/arp"
	"github.com/oecp/rama/pkg/daemon/ndp"
	daemonutils "github.com/oecp/rama/pkg/daemon/utils"
	"github.com/vishvananda/netlink"
)

func ConfigureHostNic(nicName string, allocatedIPs map[ramav1.IPVersion]*IPInfo, localDirectTableNum int) error {
	hostLink, err := netlink.LinkByName(nicName)
	if err != nil {
		return fmt.Errorf("can not find host nic %s %v", nicName, err)
	}

	if err = netlink.LinkSetUp(hostLink); err != nil {
		return fmt.Errorf("can not set host nic %s up %v", nicName, err)
	}

	macAddress, err := net.ParseMAC(ContainerHostLinkMac)
	if err != nil {
		return fmt.Errorf("parse mac %v failed: %v", ContainerHostLinkMac, err)
	}

	if err = netlink.LinkSetHardwareAddr(hostLink, macAddress); err != nil {
		return fmt.Errorf("failed to set mac address to nic %s %v", hostLink, err)
	}

	if allocatedIPs[ramav1.IPv4] != nil {
		// Enable proxy ARP, this makes the host respond to all ARP requests with its own
		// MAC.  This has a couple of advantages:
		//
		// - For containers, we install explicit routes into the containers network
		//   namespace and we use a link-local address for the gateway.  Turing on proxy ARP
		//   means that we don't need to assign the link local address explicitly to each
		//   host side of the veth, which is one fewer thing to maintain and one fewer
		//   thing we may clash over.
		sysctlPath := fmt.Sprintf(ProxyArpSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("set sysctl parameter %v failed: %v", sysctlPath, err)
		}

		// Enable routing to localhost.  This is required to allow for NAT to the local
		// host.
		sysctlPath = fmt.Sprintf(RouteLocalNetSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("set sysctl parameter %v failed: %v", sysctlPath, err)
		}

		// Normally, the kernel has a delay before responding to proxy ARP but we know
		// that's not needed in a Rama network so we disable it.
		sysctlPath = fmt.Sprintf(ProxyDelaySysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
			return fmt.Errorf("set sysctl parameter %v failed: %v", sysctlPath, err)
		}

		// Enable IP forwarding of packets coming _from_ this interface.  For packets to
		// be forwarded in both directions we need this flag to be set on the fabric-facing
		// interface too (or for the global default to be set).
		sysctlPath = fmt.Sprintf(IPv4ForwardingSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("set sysctl parameter %v failed: %v", sysctlPath, err)
		}

		mask := net.IPMask(net.ParseIP(DefaultIP4Mask).To4())
		localPodRoute := &netlink.Route{
			LinkIndex: hostLink.Attrs().Index,
			Dst: &net.IPNet{
				IP:   allocatedIPs[ramav1.IPv4].Addr,
				Mask: mask,
			},
			Table: localDirectTableNum,
		}

		if err := netlink.RouteReplace(localPodRoute); err != nil {
			return fmt.Errorf("add route %v failed: %v", localPodRoute.String(), err)
		}
	}

	if allocatedIPs[ramav1.IPv6] != nil {

		// Enable proxy NDP, similarly to proxy ARP, described above.
		//
		// But only proxy_ndp be set cannot work, proxy neigh entries should also be added
		// for each ip to proxy.
		sysctlPath := fmt.Sprintf(ProxyNdpSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("set sysctl parameter %v failed: %v", sysctlPath, err)
		}

		// Enable IP forwarding of packets coming _from_ this interface.  For packets to
		// be forwarded in both directions we need this flag to be set on the fabric-facing
		// interface too (or for the global default to be set).
		sysctlPath = fmt.Sprintf(IPv6ForwardingSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("set sysctl parameter %v failed: %v", sysctlPath, err)
		}

		mask := net.IPMask(net.ParseIP(DefaultIP6Mask).To16())
		localPodRoute := &netlink.Route{
			LinkIndex: hostLink.Attrs().Index,
			Dst: &net.IPNet{
				IP:   allocatedIPs[ramav1.IPv6].Addr,
				Mask: mask,
			},
			Table: localDirectTableNum,
		}

		if err := netlink.RouteReplace(localPodRoute); err != nil {
			return fmt.Errorf("add route %v failed: %v", localPodRoute.String(), err)
		}

		if err := netlink.NeighAdd(&netlink.Neigh{
			LinkIndex: hostLink.Attrs().Index,
			Family:    netlink.FAMILY_V6,
			Flags:     netlink.NTF_PROXY,
			IP:        allocatedIPs[ramav1.IPv6].Gw,
		}); err != nil {
			return fmt.Errorf("add neigh for ip %v/%v failed: %v", allocatedIPs[ramav1.IPv6].Gw.String(),
				hostLink.Attrs().Name, err)
		}
	}

	return nil
}

// ipAddr is a CIDR notation IP address and prefix length
func ConfigureContainerNic(nicName, nodeIfName string, allocatedIPs map[ramav1.IPVersion]*IPInfo,
	macAddr net.HardwareAddr, vlanID *uint32, netns ns.NetNS, mtu int, vlanCheckTimeout time.Duration,
	networkType ramav1.NetworkType) error {

	var defaultRouteNets []*types.Route
	var ipConfigs []*current.IPConfig

	ipv4AddressAllocated := false
	ipv6AddressAllocated := false

	var vlanIf *net.Interface
	if networkType == ramav1.NetworkTypeUnderlay {
		vlanIfName, err := EnsureVlanIf(nodeIfName, vlanID)
		if err != nil {
			return fmt.Errorf("ensure vlan interface %v error: %v", vlanIfName, err)
		}

		vlanIf, err = net.InterfaceByName(vlanIfName)
		if err != nil {
			return fmt.Errorf("get interface by name %v failed: %v", vlanIfName, err)
		}
	}

	if allocatedIPs[ramav1.IPv4] != nil {

		ipv4AddressAllocated = true
		// ipv4 address
		defaultRouteNets = append(defaultRouteNets, &types.Route{
			Dst: net.IPNet{IP: net.ParseIP("0.0.0.0").To4(), Mask: net.CIDRMask(0, 32)},
			GW:  allocatedIPs[ramav1.IPv4].Gw,
		})

		ipConfigs = append(ipConfigs, &current.IPConfig{
			Version: "4",
			Address: net.IPNet{
				IP:   allocatedIPs[ramav1.IPv4].Addr,
				Mask: allocatedIPs[ramav1.IPv4].Cidr.Mask,
			},
			Gateway:   allocatedIPs[ramav1.IPv4].Gw,
			Interface: current.Int(0),
		})

		if err := enableIPForward(netlink.FAMILY_V4); err != nil {
			return fmt.Errorf("failed to enable ipv4 forwarding: %v", err)
		}

		if err := ensureRpFilterConfigs(); err != nil {
			return fmt.Errorf("failed to ensure sysctl config: %v", err)
		}

		// Underlay gw ipv4 ip should be resolved here.
		// Only underlay network need to do this.
		if networkType == ramav1.NetworkTypeUnderlay {
			if err := arp.CheckWithTimeout(vlanIf, allocatedIPs[ramav1.IPv4].Addr,
				allocatedIPs[ramav1.IPv4].Gw, vlanCheckTimeout); err != nil {
				return fmt.Errorf("ipv4 vlan check failed: %v", err)
			}
		}
	}

	if allocatedIPs[ramav1.IPv6] != nil {

		ipv6AddressAllocated = true
		// ipv6 address
		defaultRouteNets = append(defaultRouteNets, &types.Route{
			Dst: net.IPNet{IP: net.ParseIP("::").To16(), Mask: net.CIDRMask(0, 128)},
			GW:  allocatedIPs[ramav1.IPv6].Gw,
		})

		ipConfigs = append(ipConfigs, &current.IPConfig{
			Version: "6",
			Address: net.IPNet{
				IP:   allocatedIPs[ramav1.IPv6].Addr,
				Mask: allocatedIPs[ramav1.IPv6].Cidr.Mask,
			},
			Gateway:   allocatedIPs[ramav1.IPv6].Gw,
			Interface: current.Int(0),
		})

		if err := enableIPForward(netlink.FAMILY_V6); err != nil {
			return fmt.Errorf("failed to enable ipv6 forwarding: %v", err)
		}

		if networkType == ramav1.NetworkTypeUnderlay {
			if err := ndp.CheckWithTimeout(vlanIf, allocatedIPs[ramav1.IPv6].Addr,
				allocatedIPs[ramav1.IPv6].Gw, vlanCheckTimeout); err != nil {
				return fmt.Errorf("ipv6 vlan check failed: %v", err)
			}
		}
	}

	if err := ns.WithNetNSPath(netns.Path(), func(_ ns.NetNS) error {
		containerLink, err := netlink.LinkByName(nicName)
		if err != nil {
			return fmt.Errorf("can not find container nic %s %v", nicName, err)
		}

		if err = netlink.LinkSetName(containerLink, ContainerNicName); err != nil {
			return err
		}

		link, err := netlink.LinkByName(ContainerNicName)
		if err != nil {
			return err
		}
		containerInterface := &current.Interface{
			Name:    link.Attrs().Name,
			Mac:     link.Attrs().HardwareAddr.String(),
			Sandbox: netns.Path(),
		}

		result := &current.Result{}
		result.IPs = ipConfigs
		result.Interfaces = []*current.Interface{containerInterface}
		result.Routes = defaultRouteNets

		// By default, the kernel does duplicate address detection for the IPv6 address. DAD delays use of the
		// IP for up to a second and we don't need it because it's a point-to-point link.
		//
		// This must be done before we set the links UP.
		if ipv6AddressAllocated {
			sysctlPath := fmt.Sprintf(AcceptDADSysctl, ContainerNicName)
			if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
				return fmt.Errorf("set sysctl parameter %s to %v, failed: %v", sysctlPath, 0, err)
			}
		}

		if err := ipam.ConfigureIface(ContainerNicName, result); err != nil {
			return fmt.Errorf("config container nic failed %v", err)
		}

		// IPv6 subnet direct route should not be configured here, for proxy_ndp usage described above.
		if ipv6AddressAllocated {
			v6RouteList, err := netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{
				Dst: allocatedIPs[ramav1.IPv6].Cidr,
			}, netlink.RT_FILTER_DST)
			if err != nil {
				return fmt.Errorf("list container ipv6 route failed: %v", err)
			}

			for _, route := range v6RouteList {
				if err := netlink.RouteDel(&route); err != nil {
					return fmt.Errorf("del ipv6 cidr route %v failed: %v", route.String(), err)
				}
			}
		}

		// Also delete ipv4 subnet direct route for consistency.
		if ipv4AddressAllocated {
			v4RouteList, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
				Dst: allocatedIPs[ramav1.IPv4].Cidr,
			}, netlink.RT_FILTER_DST)
			if err != nil {
				return fmt.Errorf("list container ipv4 route failed: %v", err)
			}

			for _, route := range v4RouteList {
				if err := netlink.RouteDel(&route); err != nil {
					return fmt.Errorf("del ipv4 cidr route %v failed: %v", route.String(), err)
				}
			}
		}

		if err = netlink.LinkSetHardwareAddr(link, macAddr); err != nil {
			return fmt.Errorf("can not set mac address to nic %s %v", link, err)
		}

		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return fmt.Errorf("can not set nic %s mtu %v", link, err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func GenerateContainerVethPair(containerID string) (string, string) {
	return fmt.Sprintf("%s%s", containerID[0:12], ContainerHostLinkSuffix), fmt.Sprintf("%s%s", containerID[0:12], ContainerInitLinkSuffix)
}

func ensureRpFilterConfigs() error {
	existInterfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("error get exist interfaces on system: %v", err)
	}

	for _, key := range []string{"default", "all"} {
		sysctlPath := fmt.Sprintf(RpFilterSysctl, key)
		if err = daemonutils.SetSysctl(sysctlPath, 0); err != nil {
			return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
		}
	}

	for _, existIf := range existInterfaces {
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
