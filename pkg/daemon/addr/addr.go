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

package addr

import (
	"fmt"
	"net"

	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type subnetToPodMap map[string]net.IP

type Manager struct {
	family        int
	localNodeName string

	// one valid local pod to one subnet and one local vlan interface name
	interfaceToSubnetMap map[string]subnetToPodMap
}

func CreateAddrManager(family int, nodeName string) *Manager {
	return &Manager{
		family:               family,
		localNodeName:        nodeName,
		interfaceToSubnetMap: map[string]subnetToPodMap{},
	}
}

func (m *Manager) ResetInfos() {
	m.interfaceToSubnetMap = map[string]subnetToPodMap{}
}

func (m *Manager) TryAddPodInfo(forwardNodeIfName string, subnet *net.IPNet, podIP net.IP) {
	if subnetMap := m.interfaceToSubnetMap[forwardNodeIfName]; subnetMap == nil {
		m.interfaceToSubnetMap[forwardNodeIfName] = subnetToPodMap{}
	}

	// we only need one local pod ip for every subnet
	if _, exist := m.interfaceToSubnetMap[forwardNodeIfName][subnet.String()]; !exist {
		m.interfaceToSubnetMap[forwardNodeIfName][subnet.String()] = podIP
	}
}

// SyncAddresses try to add an "enhanced" addresses on vlan node forward interface
// For some environments, physical router or switcher might check the sender address
// of arp request, if the sender ip address is not in the same subnet of target address
// the arp request will be take as invalid and dropped.
//
// So we will always keep an valid local pod address in the vlan interface without local routes.
func (m *Manager) SyncAddresses(getIPInstanceByAddress func(net.IP) (*networkingv1.IPInstance, error)) error {
	// clear all invalid enhanced addresses
	linkList, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("failed to list link: %v", err)
	}

	existEnhancedAddrMap := map[string]map[string]netlink.Addr{}
	existManualAddrSubnetMap := map[string]map[string]bool{}
	existLinkMap := map[string]netlink.Link{}

	for _, link := range linkList {
		// ignore container network virtual interfaces
		if containernetwork.CheckIfContainerNetworkLink(link.Attrs().Name) {
			continue
		}

		addrList, err := netlink.AddrList(link, m.family)
		if err != nil {
			return fmt.Errorf("failed to list addresses for link %v: %v", link.Attrs().Name, err)
		}

		for _, addr := range addrList {
			isEnhancedAddr, err := checkIfEnhancedAddr(link, addr, m.family)
			if err != nil {
				return fmt.Errorf("failed to check addr %v enhanced address: %v", addr.String(), err)
			}

			linkName := link.Attrs().Name
			cidr := ip.Network(addr.IPNet)

			if isEnhancedAddr {
				if existEnhancedAddrMap[linkName] == nil {
					existEnhancedAddrMap[linkName] = map[string]netlink.Addr{}
				}
				existEnhancedAddrMap[linkName][cidr.String()] = addr
			} else {
				if existManualAddrSubnetMap[linkName] == nil {
					existManualAddrSubnetMap[linkName] = map[string]bool{}
				}
				existManualAddrSubnetMap[linkName][cidr.String()] = true
			}
		}

		existLinkMap[link.Attrs().Name] = link
	}

	// clear enhanced addresses which are impossible to be used
	for existLinkName, existSubnetMap := range existEnhancedAddrMap {
		if targetSubnetMap, exist := m.interfaceToSubnetMap[existLinkName]; !exist {
			// link doesn't need enhanced address any more
			for _, enhancedAddr := range existSubnetMap {
				if err := netlink.AddrDel(existLinkMap[existLinkName], &enhancedAddr); err != nil {
					return fmt.Errorf("failed to delete link enhanced addr %v: %v", enhancedAddr.String(), err)
				}
			}
		} else {
			// subnet doesn't need enhanced address any more
			for subnetString, enhancedAddr := range existSubnetMap {
				if _, exist := targetSubnetMap[subnetString]; !exist {
					if err := netlink.AddrDel(existLinkMap[existLinkName], &enhancedAddr); err != nil {
						return fmt.Errorf("failed to delete link subnet enhanced addr %v : %v", enhancedAddr.String(), err)
					}
				}
			}
		}
	}

	// ensure all needed enhanced addresses
	for forwardNodeIfName, targetSubnetMap := range m.interfaceToSubnetMap {
		forwardNodeIf, err := netlink.LinkByName(forwardNodeIfName)
		if err != nil {
			return fmt.Errorf("failed to find interface %v: %v", forwardNodeIfName, err)
		}

		for subnetString, podIP := range targetSubnetMap {
			var outOfDateEnhancedAddr *netlink.Addr

			// check if manual address exist for subnet, if exist, don't do anything
			if _, exist := existManualAddrSubnetMap[forwardNodeIfName]; exist {
				if _, exist := existManualAddrSubnetMap[forwardNodeIfName][subnetString]; exist {
					// When add a new address to an interface with old addresses exist, and mask length
					// of all address are different, new address will never become a secondary address.
					continue
				}
			}

			if _, exist := existEnhancedAddrMap[forwardNodeIfName]; exist {
				// subnet enhanced address already exists
				if _, exist := existEnhancedAddrMap[forwardNodeIfName][subnetString]; exist {
					// if forward node if has exist enhanced address which is in the same subnet with target pod ip
					if enhancedAddr, exist := existEnhancedAddrMap[forwardNodeIfName][subnetString]; exist {
						// enhanced address attempt to add is the same as origin
						if enhancedAddr.IP.Equal(podIP) {
							continue
						}

						// check if exist enhanced address is valid
						ipInstance, err := getIPInstanceByAddress(enhancedAddr.IP)
						if err != nil {
							return fmt.Errorf("failed to get ip instance by address %v: %v", enhancedAddr.IP.String(), err)
						}

						if ipInstance != nil {
							nodeName := ipInstance.Labels[constants.LabelNode]
							if nodeName == m.localNodeName {
								// exist enhanced address is still valid, just keep it
								continue
							}
						}

						// ip instance not found or is no longer in this node, need to be refreshed
						outOfDateEnhancedAddr = &enhancedAddr
					}
				}
			}

			_, subnetCidr, err := net.ParseCIDR(subnetString)
			if err != nil {
				return fmt.Errorf("failed to parse subnet cidr %v: %v", subnetString, err)
			}

			if err := ensureSubnetEnhancedAddr(forwardNodeIf, &netlink.Addr{
				IPNet: &net.IPNet{
					IP:   podIP,
					Mask: subnetCidr.Mask,
				},
				Label: "",
				Flags: unix.IFA_F_NOPREFIXROUTE,
			}, outOfDateEnhancedAddr, m.family); err != nil {
				return fmt.Errorf("failed to ensure subnet enhanced addr %v: %v", podIP.String(), err)
			}
		}
	}

	return nil
}
