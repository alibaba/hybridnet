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
	for name := range n {
		names = append(names, name)
	}
	return names
}

func NewNetwork(name string, netID *uint32, lastAllocatedSubnet, lastAllocatedIPv6Subnet string, networkType NetworkType) *Network {
	return &Network{
		Name:                    name,
		NetID:                   netID,
		LastAllocatedSubnet:     lastAllocatedSubnet,
		LastAllocatedIPv6Subnet: lastAllocatedIPv6Subnet,
		Type:                    networkType,
		Subnets:                 NewSubnetSlice(),
	}
}

func (n *Network) AddSubnet(subnet *Subnet, ips IPSet) error {
	// TODO: separate IPv4 and IPv6 subnets registration
	return n.Subnets.AddSubnet(subnet, n.NetID, ips, subnet.Name == n.LastAllocatedSubnet || subnet.Name == n.LastAllocatedIPv6Subnet)
}

func (n *Network) GetSubnetByName(subnetName string) (*Subnet, error) {
	if len(subnetName) == 0 {
		return nil, fmt.Errorf("subnet name must be specified")
	}
	return n.Subnets.GetSubnet(subnetName)
}

func (n *Network) GetSubnetByNameOrIP(subnetName, ip string) (*Subnet, error) {
	if len(subnetName) > 0 {
		return n.Subnets.GetSubnet(subnetName)
	}

	return n.Subnets.GetSubnetByIP(ip)
}

func (n *Network) GetIPv4SubnetByNameOrAvailable(subnetName string) (sn *Subnet, err error) {
	defer func() {
		if sn != nil {
			n.LastAllocatedSubnet = sn.Name
		}
	}()

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

func (n *Network) GetIPv6SubnetByNameOrAvailable(subnetName string) (sn *Subnet, err error) {
	defer func() {
		if sn != nil {
			n.LastAllocatedIPv6Subnet = sn.Name
		}
	}()

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

func (n *Network) GetDualStackSubnetsByNameOrAvailable(v4SubnetName, v6SubnetName string) (v4Subnet *Subnet, v6Subnet *Subnet, err error) {
	defer func() {
		if v4Subnet != nil {
			n.LastAllocatedSubnet = v4Subnet.Name
		}
		if v6Subnet != nil {
			n.LastAllocatedIPv6Subnet = v6Subnet.Name
		}
	}()

	if len(v4SubnetName) == 0 && len(v6SubnetName) == 0 {
		return n.Subnets.GetAvailableDualStackSubnets()
	}
	if v4Subnet, err = n.GetIPv4SubnetByNameOrAvailable(v4SubnetName); err != nil {
		return
	}
	if v6Subnet, err = n.GetIPv6SubnetByNameOrAvailable(v6SubnetName); err != nil {
		return
	}
	return
}

func (n *Network) Usage() *NetworkUsage {
	_, subnetUsages := n.Subnets.Usage()
	ipv4SubnetNames, ipv6SubnetNames := n.Subnets.Classify()

	networkUsage := &NetworkUsage{
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
	networkUsage.Usages[IPv4].LastAllocation = n.LastAllocatedSubnet

	for _, ipv6SubnetName := range ipv6SubnetNames {
		networkUsage.Usages[IPv6].Add(subnetUsages[ipv6SubnetName])
	}
	networkUsage.Usages[IPv6].LastAllocation = n.LastAllocatedIPv6Subnet

	networkUsage.Usages[DualStack].Available = utils.MinUint32(networkUsage.Usages[IPv4].Available, networkUsage.Usages[IPv6].Available)

	return networkUsage
}
