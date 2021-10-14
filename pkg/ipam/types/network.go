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

func (n *Network) GetSubnetByIP(subnetName, ip string) (*Subnet, error) {
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

func (n *Network) GetPairedDualStackSubnets(v4Name, v6Name string) (v4Subnet *Subnet, v6Subnet *Subnet, err error) {
	if len(v4Name) > 0 && len(v6Name) > 0 {
		if v4Subnet, err = n.Subnets.GetSubnet(v4Name); err != nil {
			return
		}
		if v4Subnet.IsIPv6() {
			return nil, nil, fmt.Errorf("assigned v4 subnet %s is IPv6 family", v4Name)
		}
		if v6Subnet, err = n.Subnets.GetSubnet(v6Name); err != nil {
			return
		}
		if !v6Subnet.IsIPv6() {
			return nil, nil, fmt.Errorf("assigned v6 subnet %s is IPv4 family", v6Name)
		}
		if unifyNetID(v4Subnet.NetID) != unifyNetID(v6Subnet.NetID) {
			return nil, nil, fmt.Errorf("assigned subnets %s and %s have mismatched net ID", v4Name, v6Name)
		}
		return
	}

	return n.Subnets.GetAvailablePairedDualStackSubnets()
}

func (n *Network) Usage() (*Usage, map[string]*Usage, error) {
	lastAllocatedSubnet, subnetUsages, _ := n.Subnets.Usage()

	networkUsage := new(Usage)

	for _, su := range subnetUsages {
		networkUsage.Total += su.Total
		networkUsage.Available += su.Available
		networkUsage.Used += su.Used
	}

	networkUsage.LastAllocation = lastAllocatedSubnet

	return networkUsage, subnetUsages, nil
}

func (n *Network) DualStackUsage() ([3]*Usage, map[string]*Usage, error) {
	return n.Subnets.DualStackUsage()
}
