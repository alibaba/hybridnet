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
	"net"

	"github.com/alibaba/hybridnet/pkg/utils"
)

var (
	ErrNotFoundNetwork = errors.New("network not found")
	ErrEmptySubnetName = errors.New("subnet name must be specified")
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

func (n NetworkSet) GetNetworkByName(name string) (*Network, error) {
	if network, exist := n[name]; exist {
		return network, nil
	}

	return nil, ErrNotFoundNetwork
}

func (n NetworkSet) CheckNetworkByType(networkName string, networkType NetworkType) bool {
	for name, network := range n {
		if name == networkName {
			return network.Type == networkType
		}
	}
	return false
}

func (n NetworkSet) ListNetworkToNames() []string {
	var names []string
	for name := range n {
		names = append(names, name)
	}
	return names
}

func NewNetwork(name string, netID *uint32, lastAllocatedSubnet, lastAllocatedIPv6Subnet string, networkType NetworkType) *Network {
	return &Network{
		Name:        name,
		NetID:       netID,
		Type:        networkType,
		IPv4Subnets: NewSubnetSlice(lastAllocatedSubnet),
		IPv6Subnets: NewSubnetSlice(lastAllocatedIPv6Subnet),
	}
}

func (n *Network) AddSubnet(subnet *Subnet, ips IPSet) error {
	if subnet.IsIPv6() {
		return n.IPv6Subnets.AddSubnet(subnet, n.NetID, ips)
	}
	return n.IPv4Subnets.AddSubnet(subnet, n.NetID, ips)
}

func (n *Network) GetSubnetByName(subnetName string) (*Subnet, error) {
	if len(subnetName) == 0 {
		return nil, ErrEmptySubnetName
	}

	// try to get subnet by name in IPv6 after IPv4 subnets
	if subnet, err := n.IPv4Subnets.GetSubnet(subnetName); err == nil {
		return subnet, nil
	}
	return n.IPv6Subnets.GetSubnet(subnetName)
}

func (n *Network) GetSubnetByNameOrIP(subnetName, ip string) (*Subnet, error) {
	if len(subnetName) > 0 {
		return n.GetSubnetByName(subnetName)
	}

	// validate IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("%s is not a valid IP", ip)
	}

	if parsedIP.To4() == nil {
		return n.IPv6Subnets.GetSubnetByIP(ip)
	}
	return n.IPv4Subnets.GetSubnetByIP(ip)
}

func (n *Network) GetIPv4SubnetByNameOrAvailable(subnetName string) (sn *Subnet, err error) {
	if len(subnetName) > 0 {
		if sn, err = n.IPv4Subnets.GetSubnet(subnetName); err != nil {
			return nil, err
		}
		if sn.IsIPv6() {
			return nil, fmt.Errorf("assigned subnet %s is not IPv4 family", subnetName)
		}
		return
	}

	return n.IPv4Subnets.GetAvailableSubnet()
}

func (n *Network) GetIPv6SubnetByNameOrAvailable(subnetName string) (sn *Subnet, err error) {
	if len(subnetName) > 0 {
		if sn, err = n.IPv6Subnets.GetSubnet(subnetName); err != nil {
			return nil, err
		}
		if !sn.IsIPv6() {
			return nil, fmt.Errorf("assigned subnet %s is not IPv6 family", subnetName)
		}
		return
	}

	return n.IPv6Subnets.GetAvailableSubnet()
}

func (n *Network) GetDualStackSubnetsByNameOrAvailable(v4SubnetName, v6SubnetName string) (v4Subnet *Subnet, v6Subnet *Subnet, err error) {
	if v4Subnet, err = n.GetIPv4SubnetByNameOrAvailable(v4SubnetName); err != nil {
		return
	}
	if v6Subnet, err = n.GetIPv6SubnetByNameOrAvailable(v6SubnetName); err != nil {
		return
	}
	return
}

func (n *Network) Usage() *NetworkUsage {
	networkUsage := &NetworkUsage{
		Usages: map[IPFamilyMode]*Usage{
			IPv4:      n.IPv4Subnets.Usage(),
			IPv6:      n.IPv6Subnets.Usage(),
			DualStack: {},
		},
	}

	networkUsage.Usages[DualStack].Available = utils.MinUint32(networkUsage.Usages[IPv4].Available, networkUsage.Usages[IPv6].Available)
	return networkUsage
}

func (n *Network) SubnetCount() int {
	return n.IPv4Subnets.Count() + n.IPv6Subnets.Count()
}
