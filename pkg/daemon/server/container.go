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

package server

import (
	"fmt"
	"net"

	"github.com/containernetworking/plugins/pkg/ns"
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	"github.com/vishvananda/netlink"

	"k8s.io/klog"
)

// ipAddr is a CIDR notation IP address and prefix length
func (cdh cniDaemonHandler) configureNic(podName, podNamespace, netns, containerID, mac string,
	vlanID *uint32, allocatedIPs map[ramav1.IPVersion]*containernetwork.IPInfo, networkType ramav1.NetworkType) (string, error) {

	var err error
	var nodeIfName string
	var mtu int

	switch networkType {
	case ramav1.NetworkTypeUnderlay:
		mtu = cdh.config.VlanMTU
		nodeIfName = cdh.config.NodeVlanIfName
	case ramav1.NetworkTypeOverlay:
		mtu = cdh.config.VxlanMTU
		nodeIfName = cdh.config.NodeVxlanIfName
	}

	hostNicName, containerNicName := containernetwork.GenerateContainerVethPair(containerID)
	veth := netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: hostNicName, MTU: mtu}, PeerName: containerNicName}
	if err = netlink.LinkAdd(&veth); err != nil {
		return "", fmt.Errorf("create veth pair for pod %v failed: %v", podName, err)
	}

	defer func() {
		// Remove veth link in case any error during creating pod network.
		if err != nil {
			_ = netlink.LinkDel(&veth)
		}
	}()

	macAddr, err := net.ParseMAC(mac)
	if err != nil {
		return "", fmt.Errorf("failed to parse mac %s %v", macAddr, err)
	}
	podNS, err := ns.GetNS(netns)
	if err != nil {
		return "", fmt.Errorf("failed to open netns %q: %v", netns, err)
	}

	if err = containernetwork.ConfigureHostNic(hostNicName, allocatedIPs, cdh.config.LocalDirectTableNum); err != nil {
		return "", err
	}

	klog.Infof("Configure container nic for %v.%v", podName, podNamespace)
	if err = containernetwork.ConfigureContainerNic(containerNicName, nodeIfName,
		allocatedIPs, macAddr, vlanID, podNS, mtu, cdh.config.VlanCheckTimeout, networkType); err != nil {
		return "", fmt.Errorf("failed to configure container nic for %v.%v: %v", podName, podNamespace, err)
	}

	klog.Infof("Finish configuring container nic for %v.%v", podName, podNamespace)

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
		if err != nil {
			return fmt.Errorf("can not find container nic %s error: %v", containernetwork.ContainerNicName, err)
		}

		addrs, err := netlink.AddrList(containerLink, netlink.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("list addrs container nic %s error: %v", containernetwork.ContainerNicName, err)
		}

		if len(addrs) == 0 {
			return nil
		}

		// Set link down and remove ip addresses on it, in case the netns is used to do arp proxy.
		if err := netlink.LinkSetDown(containerLink); err != nil {
			return fmt.Errorf("set delete ns %v %v error: %v", netns, containernetwork.ContainerNicName, err)
		}

		for _, addr := range addrs {
			if err := netlink.AddrDel(containerLink, &addr); err != nil {
				return fmt.Errorf("delete ns %v %v addr %v error: %v", netns, containernetwork.ContainerNicName, addr.IP, err)
			}
		}
		return nil
	})
}
