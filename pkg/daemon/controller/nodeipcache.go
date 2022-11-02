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

package controller

import (
	"fmt"
	"net"
	"sync"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
)

type NodeIPCache struct {
	// ip string to node vtep mac
	nodeIPMap map[string]net.HardwareAddr
	mu        sync.RWMutex
}

func NewNodeIPCache() *NodeIPCache {
	return &NodeIPCache{
		nodeIPMap: map[string]net.HardwareAddr{},
		mu:        sync.RWMutex{},
	}
}

func (nic *NodeIPCache) UpdateNodeIPs(nodeInfoList []networkingv1.NodeInfo, localNodeName string, remoteVtepList []*multiclusterv1.RemoteVtep) error {
	nic.mu.Lock()
	defer nic.mu.Unlock()

	nic.nodeIPMap = map[string]net.HardwareAddr{}

	for _, nodeInfo := range nodeInfoList {
		// Only update remote node vtep information.
		if nodeInfo.Name == localNodeName {
			continue
		}

		// ignore empty information
		if nodeInfo.Spec.VTEPInfo == nil ||
			len(nodeInfo.Spec.VTEPInfo.IP) == 0 ||
			len(nodeInfo.Spec.VTEPInfo.LocalIPs) == 0 {
			continue
		}

		macAddr, err := net.ParseMAC(nodeInfo.Spec.VTEPInfo.MAC)
		if err != nil {
			return fmt.Errorf("failed to parse node vtep mac %v: %v", nodeInfo.Spec.VTEPInfo.MAC, err)
		}

		for _, ipString := range nodeInfo.Spec.VTEPInfo.LocalIPs {
			nic.nodeIPMap[ipString] = macAddr
		}
	}

	for _, remoteVtep := range remoteVtepList {
		macAddr, err := net.ParseMAC(remoteVtep.Spec.VTEPInfo.MAC)
		if err != nil {
			return fmt.Errorf("failed to parse remote node vtep mac %v: %v", remoteVtep.Spec.VTEPInfo.MAC, err)
		}

		if len(remoteVtep.Spec.VTEPInfo.LocalIPs) == 0 {
			nic.nodeIPMap[remoteVtep.Spec.VTEPInfo.IP] = macAddr
			continue
		}

		for _, ipString := range remoteVtep.Spec.VTEPInfo.LocalIPs {
			nic.nodeIPMap[ipString] = macAddr
		}
	}

	return nil
}

func (nic *NodeIPCache) SearchIP(ip net.IP) (net.HardwareAddr, bool) {
	nic.mu.RLock()
	defer nic.mu.RUnlock()

	mac, exist := nic.nodeIPMap[ip.String()]
	return mac, exist
}
