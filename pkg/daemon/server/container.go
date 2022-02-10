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

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
)

// ipAddr is a CIDR notation IP address and prefix length
func (cdh cniDaemonHandler) configureNic(podName, podNamespace, netns, containerID, mac string,
	netID *int32, allocatedIPs map[networkingv1.IPVersion]*containernetwork.IPInfo,
	networkMode networkingv1.NetworkMode) (string, error) {

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
	case networkingv1.NetworkModeBGP:
		mtu = cdh.config.BGPMTU
		nodeIfName = cdh.config.NodeBGPIfName
	}

	macAddr, err := net.ParseMAC(mac)
	if err != nil {
		return "", fmt.Errorf("failed to parse mac %s %v", macAddr, err)
	}

	containerNicName, hostNicName, podNS, err := initContainerNic(podName, netns, containerID, mtu)
	if err != nil {
		return "", fmt.Errorf("failed to init container nic for pod %v: %v", podName, err)
	}

	if err = containernetwork.ConfigureHostNic(hostNicName, allocatedIPs, cdh.config.LocalDirectTableNum); err != nil {
		return "", err
	}

	if err = containernetwork.ConfigureContainerNic(containerNicName, hostNicName, nodeIfName,
		allocatedIPs, macAddr, netID, podNS, mtu, cdh.config.VlanCheckTimeout, networkMode,
		cdh.config.NeighGCThresh1, cdh.config.NeighGCThresh2, cdh.config.NeighGCThresh3); err != nil {
		return "", fmt.Errorf("failed to configure container nic for %v.%v: %v", podName, podNamespace, err)
	}

	return hostNicName, nil
}

func (cdh cniDaemonHandler) deleteNic(netns string) error {
	if netns == "" {
		return nil
	}

	nsHandler, err := ns.GetNS(netns)
	if err != nil {
		return fmt.Errorf("get ns error: %v", err)
	}

	return nsHandler.Do(func(netNS ns.NetNS) error {
		containerLink, err := netlink.LinkByName(containernetwork.ContainerNicName)
		// return nil if eth0 not found.
		if err != nil {
			return nil
		}

		addrList, err := netlink.AddrList(containerLink, netlink.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("list addrs container nic %s error: %v", containernetwork.ContainerNicName, err)
		}

		if len(addrList) == 0 {
			return nil
		}

		if err := netlink.LinkSetDown(containerLink); err != nil {
			return fmt.Errorf("set delete ns %v %v error: %v", netns, containernetwork.ContainerNicName, err)
		}

		for _, addr := range addrList {
			if err := netlink.AddrDel(containerLink, &addr); err != nil {
				return fmt.Errorf("delete ns %v %v addr %v error: %v", netns, containernetwork.ContainerNicName, addr.IP, err)
			}
		}
		return nil
	})
}

func initContainerNic(podName, netns, containerID string, mtu int) (string, string, ns.NetNS, error) {
	podNS, err := ns.GetNS(netns)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to open netns %q: %v", netns, err)
	}

	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to open current namespace: %v", err)
	}

	hostNicName, containerNicName := containernetwork.GenerateContainerVethPair(containerID)

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
			return fmt.Errorf("failed to link netns %v", err)
		}

		return nil
	}); err != nil {
		return "", "", nil, fmt.Errorf("failed to generate veth pair for pod %v: %v", podName, err)
	}

	return containerNicName, hostNicName, podNS, nil
}
