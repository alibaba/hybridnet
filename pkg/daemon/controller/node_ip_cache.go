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
	"strings"
	"sync"

	multiclusterv1 "github.com/alibaba/hybridnet/apis/multicluster/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	corev1 "k8s.io/api/core/v1"
)

type NodeIPCache struct {
	// ip string to node vtep mac
	nodeIPMap map[string]net.HardwareAddr
	mu        *sync.RWMutex
}

func NewNodeIPCache() *NodeIPCache {
	return &NodeIPCache{
		nodeIPMap: map[string]net.HardwareAddr{},
		mu:        &sync.RWMutex{},
	}
}

func (nic *NodeIPCache) UpdateNodeIPs(nodeList []corev1.Node, localNodeName string, remoteVtepList []*multiclusterv1.RemoteVtep) error {
	nic.mu.Lock()
	defer nic.mu.Unlock()

	nic.nodeIPMap = map[string]net.HardwareAddr{}

	for _, node := range nodeList {
		// Only update remote node vtep information.
		if node.Name == localNodeName {
			continue
		}

		// ignore empty information
		if _, exist := node.Annotations[constants.AnnotationNodeVtepMac]; !exist {
			continue
		}
		if _, exist := node.Annotations[constants.AnnotationNodeLocalVxlanIPList]; !exist {
			continue
		}

		macAddr, err := net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
		if err != nil {
			return fmt.Errorf("failed to parse node vtep mac %v: %v", node.Annotations[constants.AnnotationNodeVtepMac], err)
		}

		ipStringList := strings.Split(node.Annotations[constants.AnnotationNodeLocalVxlanIPList], ",")
		for _, ipString := range ipStringList {
			nic.nodeIPMap[ipString] = macAddr
		}
	}

	for _, remoteVtep := range remoteVtepList {
		macAddr, err := net.ParseMAC(remoteVtep.Spec.VTEPInfo.MAC)
		if err != nil {
			return fmt.Errorf("failed to parse remote node vtep mac %v: %v", remoteVtep.Spec.VTEPInfo.MAC, err)
		}

		if _, exist := remoteVtep.Annotations[constants.AnnotationNodeLocalVxlanIPList]; !exist {
			nic.nodeIPMap[remoteVtep.Spec.VTEPInfo.IP] = macAddr
			continue
		}

		ipStringList := strings.Split(remoteVtep.Annotations[constants.AnnotationNodeLocalVxlanIPList], ",")
		for _, ipString := range ipStringList {
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
