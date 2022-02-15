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

package allocator

import (
	"fmt"
	"sync"

	"github.com/alibaba/hybridnet/pkg/ipam/types"
)

type Allocator struct {
	*sync.RWMutex

	Networks types.NetworkSet

	NetworkGetter NetworkGetter
	SubnetGetter  SubnetGetter
	IPSetGetter   IPSetGetter
}

func NewAllocator(networks []string, nGetter NetworkGetter, sGetter SubnetGetter, iGetter IPSetGetter) (*Allocator, error) {
	allocator := &Allocator{
		RWMutex:       &sync.RWMutex{},
		Networks:      types.NewNetworkSet(),
		NetworkGetter: nGetter,
		SubnetGetter:  sGetter,
		IPSetGetter:   iGetter,
	}

	if err := allocator.Refresh(networks); err != nil {
		return nil, err
	}

	return allocator, nil
}

func (a *Allocator) Refresh(networks []string) error {
	a.Lock()
	defer a.Unlock()

	for _, network := range networks {
		if err := a.refreshNetwork(network); err != nil {
			return err
		}
	}

	return nil
}

func (a *Allocator) refreshNetwork(name string) error {
	// get network spec
	network, err := a.NetworkGetter(name)
	if err != nil {
		return err
	}

	if network == nil {
		a.Networks.RemoveNetwork(name)
		return nil
	}

	// get subnets which belongs to this network
	subnets, err := a.SubnetGetter(name)
	if err != nil {
		return err
	}

	var ips types.IPSet
	for _, subnet := range subnets {
		// get using ips which belongs to this subnet
		ips, err = a.IPSetGetter(subnet.Name)
		if err != nil {
			return err
		}
		if err = network.AddSubnet(subnet, ips); err != nil {
			return err
		}
	}

	a.Networks.RefreshNetwork(name, network)

	return nil
}

func (a *Allocator) Allocate(networkName, subnetName, podName, podNamespace string) (*types.IP, error) {
	a.Lock()
	defer a.Unlock()

	network, err := a.Networks.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	subnet, err := network.GetSubnet(subnetName)
	if err != nil {
		return nil, fmt.Errorf("fail to get subnet %s: %v", subnetName, err)
	}

	availableIP := subnet.AllocateNext(podName, podNamespace)
	if availableIP == nil {
		return nil, fmt.Errorf("fail to get available ip from subnet %s", subnet.Name)
	}

	return availableIP, nil
}

// for re-use allocated ip address or use reserved ip address
func (a *Allocator) Assign(networkName, subnetName, podName, podNamespace, ip string, forced bool) (*types.IP, error) {
	a.Lock()
	defer a.Unlock()

	network, err := a.Networks.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	subnet, err := network.GetSubnetByIP(subnetName, ip)
	if err != nil {
		return nil, fmt.Errorf("fail to get subnet %s by ip %s: %v", subnetName, ip, err)
	}

	assignedIP, err := subnet.Assign(podName, podNamespace, ip, forced)
	if err != nil {
		return nil, fmt.Errorf("fail to assign ip %s in subnet %s: %v", ip, subnetName, err)
	}

	return assignedIP, nil
}

func (a *Allocator) Release(networkName, subnetName, ip string) error {
	a.Lock()
	defer a.Unlock()

	network, err := a.Networks.GetNetwork(networkName)
	if err != nil {
		return fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	subnet, err := network.GetSubnet(subnetName)
	if err != nil {
		return fmt.Errorf("fail to get subnet %s: %v", subnetName, err)
	}

	subnet.Release(ip)

	return nil
}

func (a *Allocator) Usage(networkName string) (*types.Usage, map[string]*types.Usage, error) {
	a.RLock()
	defer a.RUnlock()

	network, err := a.Networks.GetNetwork(networkName)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	return network.Usage()
}

func (a *Allocator) SubnetUsage(networkName, subnetName string) (*types.Usage, error) {
	a.RLock()
	defer a.RUnlock()

	network, err := a.Networks.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	subnet, err := network.GetSubnet(subnetName)
	if err != nil {
		return nil, fmt.Errorf("fail to get subnet %s: %v", subnetName, err)
	}

	return subnet.Usage(), nil
}

func (a *Allocator) GetNetworksByType(networkType types.NetworkType) []string {
	a.RLock()
	defer a.RUnlock()

	return a.Networks.GetNetworksByType(networkType)
}

func (a *Allocator) MatchNetworkType(networkName string, networkType types.NetworkType) bool {
	a.RLock()
	defer a.RUnlock()

	return a.Networks.MatchNetworkType(networkName, networkType)
}
