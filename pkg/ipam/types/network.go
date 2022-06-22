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

package types

import (
	"errors"
	"fmt"

	"github.com/alibaba/hybridnet/pkg/utils"
)

var (
	ErrNotFoundNetwork = errors.New("network not found")
)

func NewNetworkSet() NetworkSet {
	return make(map[string]*Network)
}

func (n NetworkSet) RefreshNetwork(name string, network *Network) {
	n[name] = network
}

func (n NetworkSet) RemoveNetwork(name string) {
	delete(n, name)
}

func (n NetworkSet) GetNetwork(name string) (*Network, error) {
	if network, exist := n[name]; exist {
		return network, nil
	}

	return nil, ErrNotFoundNetwork
}

func (n NetworkSet) GetNetworksByType(networkType NetworkType) (names []string) {
	for name, network := range n {
		if network.Type == networkType {
			names = append(names, name)
		}
	}
	return
}

func (n NetworkSet) MatchNetworkType(networkName string, networkType NetworkType) bool {
	for name, network := range n {
		if name == networkName {
			return network.Type == networkType
		}
	}
	return false
}

func (n NetworkSet) ListNetwork() []string {
	var names []string
	for name, _ := range n {
		names = append(names, name)
	}
	return names
}

func NewNetwork(name string, netID *uint32, lastAllocatedSubnet string, networkType NetworkType) *Network {
	return &Network{
		Name:                name,
		NetID:               netID,
		LastAllocatedSubnet: lastAllocatedSubnet,
		Type:                networkType,
		Subnets:             NewSubnetSlice(),
	}
}

func (n *Network) AddSubnet(subnet *Subnet, ips IPSet) error {
	return n.Subnets.AddSubnet(subnet, n.NetID, ips, subnet.Name == n.LastAllocatedSubnet)
}

func (n *Network) GetSubnet(subnetName string) (*Subnet, error) {
	if len(subnetName) > 0 {
		return n.Subnets.GetSubnet(subnetName)
	}

	return n.Subnets.GetAvailableSubnet()
}

func (n *Network) GetSubnetByNameOrIP(subnetName, ip string) (*Subnet, error) {
	if len(subnetName) > 0 {
		return n.Subnets.GetSubnet(subnetName)
	}

	return n.Subnets.GetSubnetByIP(ip)
}

func (n *Network) GetIPv4Subnet(subnetName string) (sn *Subnet, err error) {
	if len(subnetName) > 0 {
		if sn, err = n.Subnets.GetSubnet(subnetName); err != nil {
			return nil, err
		}
		if sn.IsIPv6() {
			return nil, fmt.Errorf("assigned subnet %s is IPv6 family", subnetName)
		}
		return
	}

	return n.Subnets.GetAvailableIPv4Subnet()
}

func (n *Network) GetIPv6Subnet(subnetName string) (sn *Subnet, err error) {
	if len(subnetName) > 0 {
		if sn, err = n.Subnets.GetSubnet(subnetName); err != nil {
			return nil, err
		}
		if !sn.IsIPv6() {
			return nil, fmt.Errorf("assigned subnet %s is IPv4 family", subnetName)
		}
		return
	}

	return n.Subnets.GetAvailableIPv6Subnet()
}

func (n *Network) GetDualStackSubnets(v4SubnetName, v6SubnetName string) (v4Subnet *Subnet, v6Subnet *Subnet, err error) {
	if len(v4SubnetName) == 0 && len(v6SubnetName) == 0 {
		return n.Subnets.GetAvailableDualStackSubnets()
	}
	if v4Subnet, err = n.GetIPv4Subnet(v4SubnetName); err != nil {
		return
	}
	if v6Subnet, err = n.GetIPv6Subnet(v6SubnetName); err != nil {
		return
	}
	return
}

func (n *Network) Usage() *NetworkUsage {
	lastAllocatedSubnet, subnetUsages := n.Subnets.Usage()
	ipv4SubnetNames, ipv6SubnetNames := n.Subnets.Classify()

	networkUsage := &NetworkUsage{
		LastAllocation: lastAllocatedSubnet,
		Usages: map[IPFamilyMode]*Usage{
			IPv4:      {},
			IPv6:      {},
			DualStack: {},
		},
		SubnetUsages: subnetUsages,
	}

	for _, ipv4SubnetName := range ipv4SubnetNames {
		networkUsage.Usages[IPv4].Add(subnetUsages[ipv4SubnetName])
	}
	for _, ipv6SubnetName := range ipv6SubnetNames {
		networkUsage.Usages[IPv6].Add(subnetUsages[ipv6SubnetName])
	}

	networkUsage.Usages[DualStack].Available = utils.MinUint32(networkUsage.Usages[IPv4].Available, networkUsage.Usages[IPv6].Available)

	return networkUsage
}
