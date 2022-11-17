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

package server

import (
	"fmt"
	"net"

	"github.com/containernetworking/plugins/pkg/ip"

	"github.com/alibaba/hybridnet/pkg/constants"

	"github.com/alibaba/hybridnet/pkg/daemon/utils"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
)

// ipAddr is a CIDR notation IP address and prefix length
func (cdh *cniDaemonHandler) configureNic(podName, podNamespace, netns, mac string,
	allocatedIPs map[networkingv1.IPVersion]*utils.IPInfo, networkMode networkingv1.NetworkMode) (string, error) {

	var err error
	var nodeIfName string
	var mtu int

	switch networkMode {
	case networkingv1.NetworkModeVlan:
		mtu = cdh.config.VlanMTU
		nodeIfName = cdh.config.NodeVlanIfName
	case networkingv1.NetworkModeVxlan:
		mtu = cdh.config.VxlanMTU
		nodeIfName = cdh.config.NodeVxlanIfName
	case networkingv1.NetworkModeBGP, networkingv1.NetworkModeGlobalBGP:
		mtu = cdh.config.BGPMTU
		nodeIfName = cdh.config.NodeBGPIfName
	}

	macAddr, err := net.ParseMAC(mac)
	if err != nil {
		return "", fmt.Errorf("failed to parse mac %s %v", macAddr, err)
	}

	containerNicName, hostNicName, podNS, err := initContainerNic(podName, podNamespace, netns, mtu)
	if err != nil {
		return "", fmt.Errorf("failed to init container nic for pod %v: %v", podName, err)
	}

	defer func() {
		if err != nil {
			// clean the veth pair
			_ = deleteContainerNic(netns)
		}
	}()

	if err = containernetwork.ConfigureHostNic(hostNicName, allocatedIPs, cdh.config.LocalDirectTableNum); err != nil {
		return "", fmt.Errorf("failed to configure host nic for %v.%v: %v", podName, podNamespace, err)
	}

	if err = containernetwork.ConfigureContainerNic(containerNicName, hostNicName, nodeIfName,
		allocatedIPs, macAddr, podNS, mtu, cdh.config.VlanCheckTimeout, networkMode,
		cdh.config.NeighGCThresh1, cdh.config.NeighGCThresh2, cdh.config.NeighGCThresh3, cdh.config.IPv6RouteCacheMaxSize,
		cdh.config.IPv6RouteCacheGCThresh, cdh.bgpManager); err != nil {
		return "", fmt.Errorf("failed to configure container nic for %v.%v: %v", podName, podNamespace, err)
	}

	if allocatedIPs[networkingv1.IPv4] != nil {
		podIP := allocatedIPs[networkingv1.IPv4].Addr

		if cdh.config.CheckPodConnectivityFromHost {
			// ICMP traffic from pod's node to pod is always assumed allowed.
			// If the node has an usable ip, check the local pod's connectivity from node.
			if err := containernetwork.CheckReachabilityFromHost(podIP, netlink.FAMILY_V4); err != nil {
				return "", fmt.Errorf("falied to check the connectivity of local pod ip %v: %v", podIP, err)
			}
		}
	}

	if allocatedIPs[networkingv1.IPv6] != nil {
		podIP := allocatedIPs[networkingv1.IPv6].Addr

		if cdh.config.CheckPodConnectivityFromHost {
			// ICMP traffic from pod's node to pod is always assumed allowed.
			// If the node has an usable ip, check the local pod's connectivity from node.
			if err := containernetwork.CheckReachabilityFromHost(podIP, netlink.FAMILY_V6); err != nil {
				return "", fmt.Errorf("falied to check the connectivity of local pod ip %v: %v", podIP, err)
			}
		}
	}

	return hostNicName, nil
}

func (cdh *cniDaemonHandler) deleteNic(netns string) error {
	return deleteContainerNic(netns)
}

func deleteContainerNic(netns string) error {
	nsHandler, err := ns.GetNS(netns)
	if err != nil {
		return fmt.Errorf("get ns error: %v", err)
	}
	defer nsHandler.Close()

	return nsHandler.Do(func(netNS ns.NetNS) error {
		if err := ip.DelLinkByName(constants.ContainerNicName); err != nil && err != ip.ErrLinkNotFound {
			return err
		}
		return nil
	})
}

func initContainerNic(podName, podNamespace, netns string, mtu int) (string, string, ns.NetNS, error) {
	podNS, err := ns.GetNS(netns)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer podNS.Close()

	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to get host namespace: %v", err)
	}
	defer hostNS.Close()

	hostNicName, containerNicName := containernetwork.GenerateContainerVethPair(podNamespace, podName)

	if err := ns.WithNetNSPath(podNS.Path(), func(_ ns.NetNS) error {
		veth := netlink.Veth{
			LinkAttrs: netlink.LinkAttrs{
				Name: hostNicName,
				MTU:  mtu,
			},
			PeerName: containerNicName,
		}
		if err = netlink.LinkAdd(&veth); err != nil {
			return fmt.Errorf("failed to create veth pair in netns %v for pod %v: %v", podNS.Path(), podName, err)
		}

		containerHostLink, err := netlink.LinkByName(hostNicName)
		if err != nil {
			return fmt.Errorf("can not find container host nic %s in netns %v: %v", hostNicName, podNS.Path(), err)
		}

		if err = netlink.LinkSetNsFd(containerHostLink, int(hostNS.Fd())); err != nil {
			return fmt.Errorf("failed to set link %v to host netns: %v", hostNicName, err)
		}

		return nil
	}); err != nil {
		return "", "", nil, fmt.Errorf("failed to generate veth pair for pod %v: %v", podName, err)
	}

	return containerNicName, hostNicName, podNS, nil
}
