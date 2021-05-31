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

package ipam

import (
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/labels"

	v1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/ipam/types"
)

type Cache struct {
	*sync.RWMutex

	networks      map[string]*networkInfo
	subnets       map[string]*types.Usage
	globalNetwork string
}

type networkInfo struct {
	selector    labels.Selector
	selectorRaw labels.Set

	usage          *types.Usage
	ipv6Usage      *types.Usage
	dualStackUsage *types.Usage
}

func NewCache() *Cache {
	return &Cache{
		RWMutex:  new(sync.RWMutex),
		networks: make(map[string]*networkInfo, 0),
		subnets:  make(map[string]*types.Usage, 0),
	}
}

func (n *Cache) GetNetworkList() []string {
	n.RLock()
	defer n.RUnlock()

	var list []string
	for name := range n.networks {
		list = append(list, name)
	}
	return list
}

func (n *Cache) GetGlobalNetwork() string {
	n.RLock()
	defer n.RUnlock()

	return n.globalNetwork
}

func (n *Cache) SelectNetworkByLabels(nodeLabels map[string]string) string {
	n.RLock()
	defer n.RUnlock()

	for name, network := range n.networks {
		if network.selector != nil && network.selector.Matches(labels.Set(nodeLabels)) {
			return name
		}
	}

	return ""
}

func (n *Cache) MatchNetworkByLabels(networkName string, nodeLabels map[string]string) bool {
	n.RLock()
	defer n.RUnlock()

	if network, exist := n.networks[networkName]; exist && network.selector != nil {
		return network.selector.Matches(labels.Set(nodeLabels))
	}
	return false
}

func (n *Cache) UpdateNetworkCache(network *v1.Network) {
	n.Lock()
	defer n.Unlock()

	if info, exist := n.networks[network.Name]; exist {
		if labels.Equals(info.selectorRaw, labels.Set(network.Spec.NodeSelector)) {
			return
		}
	}

	info := new(networkInfo)
	if len(network.Spec.NodeSelector) > 0 {
		info.selector = labels.SelectorFromSet(network.Spec.NodeSelector)
		info.selectorRaw = labels.Set(network.Spec.NodeSelector)
	}
	if network.Status.Statistics != nil {
		info.usage = &types.Usage{
			Total:          network.Status.Statistics.Total,
			Used:           network.Status.Statistics.Used,
			Available:      network.Status.Statistics.Available,
			LastAllocation: network.Status.LastAllocatedSubnet,
		}
	}
	if network.Status.IPv6Statistics != nil {
		info.ipv6Usage = &types.Usage{
			Total:          network.Status.IPv6Statistics.Total,
			Used:           network.Status.IPv6Statistics.Used,
			Available:      network.Status.IPv6Statistics.Available,
			LastAllocation: network.Status.LastAllocatedIPv6Subnet,
		}
	}
	if network.Status.DualStackStatistics != nil {
		info.dualStackUsage = &types.Usage{
			Available: network.Status.DualStackStatistics.Available,
		}
	}

	n.networks[network.Name] = info

	// use overlay network as global option
	if v1.GetNetworkType(network) == v1.NetworkTypeOverlay {
		n.globalNetwork = network.Name
	}
}

func (n *Cache) RemoveNetworkCache(networkName string) {
	n.Lock()
	defer n.Unlock()

	delete(n.networks, networkName)

	if n.globalNetwork == networkName {
		n.globalNetwork = ""
	}
}

func (n *Cache) UpdateNetworkUsage(networkName string, usage *types.Usage) bool {
	n.Lock()
	defer n.Unlock()

	if n, exist := n.networks[networkName]; exist && !reflect.DeepEqual(n.usage, usage) {
		n.usage = usage
		return true
	}

	return false
}

func (n *Cache) GetNetworkUsage(networkName string) *types.Usage {
	n.RLock()
	defer n.RUnlock()

	network, exist := n.networks[networkName]
	if !exist {
		return nil
	}
	return network.usage
}

func (n *Cache) UpdateNetworkUsages(networkName string, usages [3]*types.Usage) bool {
	n.Lock()
	defer n.Unlock()

	network, exist := n.networks[networkName]
	if !exist {
		return false
	}

	var toDo = false
	if !reflect.DeepEqual(network.usage, usages[0]) {
		network.usage = usages[0]
		toDo = true
	}
	if !reflect.DeepEqual(network.ipv6Usage, usages[1]) {
		network.ipv6Usage = usages[1]
		toDo = true
	}
	if !reflect.DeepEqual(network.dualStackUsage, usages[2]) {
		network.dualStackUsage = usages[2]
		toDo = true
	}

	return toDo
}

func (n *Cache) GetNetworkUsages(networkName string) (ret [3]*types.Usage) {
	n.RLock()
	defer n.RUnlock()

	ret = [3]*types.Usage{}

	network, exist := n.networks[networkName]
	if !exist {
		return
	}

	ret[0] = network.usage
	ret[1] = network.ipv6Usage
	ret[2] = network.dualStackUsage
	return
}

func (n *Cache) UpdateSubnetUsage(subnetName string, usage *types.Usage) bool {
	n.Lock()
	defer n.Unlock()

	if s, exist := n.subnets[subnetName]; exist && reflect.DeepEqual(s, usage) {
		return false
	}

	n.subnets[subnetName] = usage
	return true
}
