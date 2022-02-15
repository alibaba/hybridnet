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

package neigh

import (
	"fmt"
	"io/ioutil"
	"net"
	"strconv"

	"github.com/vishvananda/netlink"
)

type IPMap map[string]net.IP

type Manager struct {
	family int

	// forward interfaces to pod ip list
	interfaceToIPSliceMap map[string]IPMap
}

// Proxy neigh cache will be cleaned if interface is set down-up again.

func CreateNeighManager(family int) *Manager {
	return &Manager{
		family:                family,
		interfaceToIPSliceMap: make(map[string]IPMap),
	}
}

func (m *Manager) ResetInfos() {
	m.interfaceToIPSliceMap = map[string]IPMap{}
}

func (m *Manager) AddPodInfo(podIP net.IP, forwardNodeIfName string) {
	if ipMap := m.interfaceToIPSliceMap[forwardNodeIfName]; ipMap == nil {
		m.interfaceToIPSliceMap[forwardNodeIfName] = IPMap{}
	}

	m.interfaceToIPSliceMap[forwardNodeIfName][podIP.String()] = podIP
}

func (m *Manager) SyncNeighs() error {
	for forwardNodeIfName, ipMap := range m.interfaceToIPSliceMap {
		forwardNodeIf, err := netlink.LinkByName(forwardNodeIfName)
		if err != nil {
			return fmt.Errorf("failed to get forward node if %v: %v", forwardNodeIfName, err)
		}

		neighList, err := netlink.NeighProxyList(forwardNodeIf.Attrs().Index, m.family)
		if err != nil {
			return fmt.Errorf("failed to list neighs for forward node if %v: %v", forwardNodeIfName, err)
		}

		if m.family == netlink.FAMILY_V6 {
			// For ipv6, proxy_ndp need to be set.
			sysctlPath := fmt.Sprintf("/proc/sys/net/ipv6/conf/%s/proxy_ndp", forwardNodeIfName)
			if err := ioutil.WriteFile(sysctlPath, []byte(strconv.Itoa(1)), 0640); err != nil {
				return fmt.Errorf("failed to set sysctl parameter %v: %v", sysctlPath, err)
			}
		}

		existNeighMap := map[string]bool{}
		for _, neigh := range neighList {
			if _, exist := ipMap[neigh.IP.String()]; !exist {
				if err := netlink.NeighDel(&neigh); err != nil {
					return fmt.Errorf("failed to delete neigh for %v/%v: %v", neigh.IP.String(), forwardNodeIfName, err)
				}
			} else {
				existNeighMap[neigh.IP.String()] = true
			}
		}

		for _, ip := range ipMap {
			if _, exist := existNeighMap[ip.String()]; !exist {
				if err := netlink.NeighAdd(&netlink.Neigh{
					LinkIndex: forwardNodeIf.Attrs().Index,
					Family:    m.family,
					Flags:     netlink.NTF_PROXY,
					IP:        ip,
				}); err != nil {
					return fmt.Errorf("failed to add neigh for ip %v/%v: %v", ip.String(), forwardNodeIfName, err)
				}
			}
		}
	}

	return nil
}
