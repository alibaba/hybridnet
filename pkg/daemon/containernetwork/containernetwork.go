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
	"fmt"
	"net"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/vishvananda/netlink"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/arp"
	"github.com/alibaba/hybridnet/pkg/daemon/bgp"
	"github.com/alibaba/hybridnet/pkg/daemon/ndp"
	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
)

func ConfigureHostNic(nicName string, allocatedIPs map[networkingv1.IPVersion]*daemonutils.IPInfo, localDirectTableNum int) error {
	hostLink, err := netlink.LinkByName(nicName)
	if err != nil {
		return fmt.Errorf("can not find host nic %s %v", nicName, err)
	}

	if err = netlink.LinkSetUp(hostLink); err != nil {
		return fmt.Errorf("can not set host nic %s up %v", nicName, err)
	}

	macAddress, err := net.ParseMAC(constants.ContainerHostLinkMac)
	if err != nil {
		return fmt.Errorf("failed to parse mac %v: %v", constants.ContainerHostLinkMac, err)
	}

	if err = netlink.LinkSetHardwareAddr(hostLink, macAddress); err != nil {
		return fmt.Errorf("failed to set mac address to nic %s %v", hostLink, err)
	}

	if allocatedIPs[networkingv1.IPv4] != nil {
		// Enable proxy ARP, this makes the host respond to all ARP requests with its own
		// MAC.  This has a couple of advantages:
		//
		// - For containers, we install explicit routes into the containers network
		//   namespace and we use a link-local address for the gateway.  Turing on proxy ARP
		//   means that we don't need to assign the link local address explicitly to each
		//   host side of the veth, which is one fewer thing to maintain and one fewer
		//   thing we may clash over.
		sysctlPath := fmt.Sprintf(constants.ProxyArpSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		// Enable routing to localhost.  This is required to allow for NAT to the local
		// host.
		sysctlPath = fmt.Sprintf(constants.RouteLocalNetSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		// Normally, the kernel has a delay before responding to proxy ARP but we know
		// that's not needed in a Hybridnet network so we disable it.
		sysctlPath = fmt.Sprintf(constants.ProxyDelaySysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
			return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		// Enable IP forwarding of packets coming _from_ this interface.  For packets to
		// be forwarded in both directions we need this flag to be set on the fabric-facing
		// interface too (or for the global default to be set).
		sysctlPath = fmt.Sprintf(constants.IPv4ForwardingSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		mask := net.IPMask(net.ParseIP(constants.DefaultIP4Mask).To4())
		localPodRoute := &netlink.Route{
			LinkIndex: hostLink.Attrs().Index,
			Dst: &net.IPNet{
				IP:   allocatedIPs[networkingv1.IPv4].Addr,
				Mask: mask,
			},
			Table: localDirectTableNum,
		}

		if err := netlink.RouteReplace(localPodRoute); err != nil {
			return fmt.Errorf("failed to add route %v: %v", localPodRoute.String(), err)
		}
	}

	if allocatedIPs[networkingv1.IPv6] != nil {

		// Enable proxy NDP, similarly to proxy ARP, described above.
		//
		// But only proxy_ndp be set cannot work, proxy neigh entries should also be added
		// for each ip to proxy.
		sysctlPath := fmt.Sprintf(constants.ProxyNdpSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		// Enable IP forwarding of packets coming _from_ this interface.  For packets to
		// be forwarded in both directions we need this flag to be set on the fabric-facing
		// interface too (or for the global default to be set).
		sysctlPath = fmt.Sprintf(constants.IPv6ForwardingSysctl, nicName)
		if err := daemonutils.SetSysctl(sysctlPath, 1); err != nil {
			return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
		}

		mask := net.IPMask(net.ParseIP(constants.DefaultIP6Mask).To16())
		localPodRoute := &netlink.Route{
			LinkIndex: hostLink.Attrs().Index,
			Dst: &net.IPNet{
				IP:   allocatedIPs[networkingv1.IPv6].Addr,
				Mask: mask,
			},
			Table: localDirectTableNum,
		}

		if err := netlink.RouteReplace(localPodRoute); err != nil {
			return fmt.Errorf("failed to add route %v: %v", localPodRoute.String(), err)
		}

		if err := netlink.NeighAdd(&netlink.Neigh{
			LinkIndex: hostLink.Attrs().Index,
			Family:    netlink.FAMILY_V6,
			Flags:     netlink.NTF_PROXY,
			IP:        net.ParseIP(constants.PodVirtualV6DefaultGateway),
		}); err != nil {
			return fmt.Errorf("failed to add neigh for ip %v/%v: %v", constants.PodVirtualV4DefaultGateway,
				hostLink.Attrs().Name, err)
		}
	}

	return nil
}

func ConfigureContainerNic(containerNicName, hostNicName, nodeIfName string, allocatedIPs map[networkingv1.IPVersion]*daemonutils.IPInfo,
	macAddr net.HardwareAddr, netID *int32, netns ns.NetNS, mtu int, vlanCheckTimeout time.Duration,
	networkMode networkingv1.NetworkMode, neighGCThresh1, neighGCThresh2, neighGCThresh3 int, bgpManager *bgp.Manager) error {

	var defaultRouteNets []*types.Route
	var ipConfigs []*current.IPConfig
	var forwardNodeIfName string
	var err error

	ipv6AddressAllocated := false

	switch networkMode {
	case networkingv1.NetworkModeVlan:
		forwardNodeIfName, err = daemonutils.GenerateVlanNetIfName(nodeIfName, netID)
		if err != nil {
			return fmt.Errorf("failed to generate vlan forward node interface name: %v", err)
		}
	case networkingv1.NetworkModeVxlan:
		forwardNodeIfName, err = daemonutils.GenerateVxlanNetIfName(nodeIfName, netID)
		if err != nil {
			return fmt.Errorf("failed to generate vxlan forward node interface name: %v", err)
		}
	case networkingv1.NetworkModeBGP, networkingv1.NetworkModeGlobalBGP:
		forwardNodeIfName = nodeIfName
	}

	forwardNodeIf, err := net.InterfaceByName(forwardNodeIfName)
	if err != nil {
		return fmt.Errorf("failed get forward node interface %v: %v; if not exist, waiting for daemon to create it", forwardNodeIfName, err)
	}

	if allocatedIPs[networkingv1.IPv4] != nil {
		// ipv4 address
		defaultRouteNets = append(defaultRouteNets, &types.Route{
			Dst: net.IPNet{IP: net.ParseIP("0.0.0.0").To4(), Mask: net.CIDRMask(0, 32)},
			GW:  net.ParseIP(constants.PodVirtualV4DefaultGateway),
		})

		podIP := allocatedIPs[networkingv1.IPv4].Addr
		podCidr := allocatedIPs[networkingv1.IPv4].Cidr

		ipConfigs = append(ipConfigs, &current.IPConfig{
			Version: "4",
			Address: net.IPNet{
				IP:   podIP,
				Mask: podCidr.Mask,
			},
			Interface: current.Int(0),
		})

		if err := daemonutils.EnableIPForward(netlink.FAMILY_V4); err != nil {
			return fmt.Errorf("failed to enable ipv4 forwarding: %v", err)
		}

		if err := daemonutils.EnsureNeighGCThresh(netlink.FAMILY_V4, neighGCThresh1, neighGCThresh2, neighGCThresh3); err != nil {
			return fmt.Errorf("failed to ensure ipv4 neigh gc thresh: %v", err)
		}

		if err := EnsureRpFilterConfigs(hostNicName); err != nil {
			return fmt.Errorf("failed to ensure sysctl config: %v", err)
		}

		// Vlan gw ipv4 ip should be resolved here.
		// Only underlay network need to do this.
		if networkMode == networkingv1.NetworkModeVlan {
			if allocatedIPs[networkingv1.IPv4].Gw == nil {
				return fmt.Errorf("get a nil gateway for ip %v", allocatedIPs[networkingv1.IPv4].Addr)
			}

			if err := arp.CheckWithTimeout(forwardNodeIf, podIP,
				allocatedIPs[networkingv1.IPv4].Gw, vlanCheckTimeout); err != nil {
				return fmt.Errorf("failed to check ipv4 vlan environment: %v", err)
			}
		}

		if err := checkPodNetConfigReady(podIP, podCidr, forwardNodeIf.Index, netlink.FAMILY_V4,
			networkMode, bgpManager); err != nil {
			return fmt.Errorf("failed to check pod ip %v network configuration: %v", podIP, err)
		}
	}

	if allocatedIPs[networkingv1.IPv6] != nil {

		ipv6AddressAllocated = true
		// ipv6 address
		defaultRouteNets = append(defaultRouteNets, &types.Route{
			Dst: net.IPNet{IP: net.ParseIP("::").To16(), Mask: net.CIDRMask(0, 128)},
			GW:  net.ParseIP(constants.PodVirtualV6DefaultGateway),
		})

		podIP := allocatedIPs[networkingv1.IPv6].Addr
		podCidr := allocatedIPs[networkingv1.IPv6].Cidr

		ipConfigs = append(ipConfigs, &current.IPConfig{
			Version: "6",
			Address: net.IPNet{
				IP:   podIP,
				Mask: podCidr.Mask,
			},
			Interface: current.Int(0),
		})

		if err := daemonutils.EnableIPForward(netlink.FAMILY_V6); err != nil {
			return fmt.Errorf("failed to enable ipv6 forwarding: %v", err)
		}

		if err := daemonutils.EnsureNeighGCThresh(netlink.FAMILY_V6, neighGCThresh1, neighGCThresh2, neighGCThresh3); err != nil {
			return fmt.Errorf("failed to ensure ipv6 neigh gc thresh: %v", err)
		}

		if networkMode == networkingv1.NetworkModeVlan {
			if allocatedIPs[networkingv1.IPv6].Gw == nil {
				return fmt.Errorf("get a nil gateway for ip %v", allocatedIPs[networkingv1.IPv6].Addr)
			}

			if err := ndp.CheckWithTimeout(forwardNodeIf, podIP,
				allocatedIPs[networkingv1.IPv6].Gw, vlanCheckTimeout); err != nil {
				return fmt.Errorf("failed to check ipv6 vlan environment: %v", err)
			}
		}

		if err := checkPodNetConfigReady(podIP, podCidr, forwardNodeIf.Index, netlink.FAMILY_V6,
			networkMode, bgpManager); err != nil {
			return fmt.Errorf("failed to check pod ip %v network configuration: %v", podIP, err)
		}
	}

	if err := ns.WithNetNSPath(netns.Path(), func(_ ns.NetNS) error {
		containerLink, err := netlink.LinkByName(containerNicName)
		if err != nil {
			return fmt.Errorf("can not find container nic %s %v", containerNicName, err)
		}

		if err = netlink.LinkSetName(containerLink, constants.ContainerNicName); err != nil {
			return err
		}

		link, err := netlink.LinkByName(constants.ContainerNicName)
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
			sysctlPath := fmt.Sprintf(constants.AcceptDADSysctl, constants.ContainerNicName)
			if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
				return fmt.Errorf("failed to set sysctl parameter %s to %v: %v", sysctlPath, 0, err)
			}
		}

		if err := daemonutils.ConfigureIface(constants.ContainerNicName, result); err != nil {
			return fmt.Errorf("failed to config container nic: %v", err)
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
