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

package manager

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/alibaba/hybridnet/pkg/ipam"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils"
)

var _ ipam.Manager = &Manager{}

// Manager is the build-in IPAM Manager implementation
type Manager struct {
	sync.RWMutex

	Networks types.NetworkSet

	NetworkGetter NetworkGetter
	SubnetGetter  SubnetGetter
	IPSetGetter   IPSetGetter
}

// NewManager is a kind of constructor of ipam.Manager
func NewManager(networks []string, nGetter NetworkGetter, sGetter SubnetGetter, iGetter IPSetGetter) (ipam.Manager, error) {
	manager := &Manager{
		RWMutex:       sync.RWMutex{},
		Networks:      types.NewNetworkSet(),
		NetworkGetter: nGetter,
		SubnetGetter:  sGetter,
		IPSetGetter:   iGetter,
	}

	if err := manager.Refresh(types.RefreshNetworks(networks)); err != nil {
		return nil, err
	}

	return manager, nil
}

// Refresh will trigger network data update in cache
func (m *Manager) Refresh(opts ...types.RefreshOption) error {
	m.Lock()
	defer m.Unlock()

	options := &types.RefreshOptions{}
	options.ApplyOptions(opts)

	var toRefreshNetworkNames []string
	if options.ForceAll {
		toRefreshNetworkNames = m.Networks.ListNetwork()
	} else {
		toRefreshNetworkNames = options.Networks
	}

	for _, networkName := range toRefreshNetworkNames {
		if err := m.refreshNetwork(networkName); err != nil {
			return err
		}
	}

	return nil
}

// GetNetworkUsage will return a network usage statistics including different ip family
// and all subnets
func (m *Manager) GetNetworkUsage(networkName string) (*types.NetworkUsage, error) {
	m.RLock()
	defer m.RUnlock()

	validateFunctions := []func() error{
		func() error { return utils.CheckNotEmpty("network name", networkName) },
	}

	if err := errors.AggregateGoroutines(validateFunctions...); err != nil {
		return nil, fmt.Errorf("validation fail: %v", err)
	}

	var network *types.Network
	var err error
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}
	return network.Usage(), nil
}

// GetSubnetUsage will return a usage statistics of a specified subnet of a network
func (m *Manager) GetSubnetUsage(networkName, subnetName string) (*types.Usage, error) {
	m.RLock()
	defer m.RUnlock()

	validateFunctions := []func() error{
		func() error { return utils.CheckNotEmpty("network name", networkName) },
		func() error { return utils.CheckNotEmpty("subnet name", subnetName) },
	}

	if err := errors.AggregateGoroutines(validateFunctions...); err != nil {
		return nil, fmt.Errorf("validation fail: %v", err)
	}

	var network *types.Network
	var err error
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	var subnet *types.Subnet
	if subnet, err = network.GetSubnet(subnetName); err != nil {
		return nil, fmt.Errorf("fail to get subnet %s: %v", subnetName, err)
	}

	return subnet.Usage(), nil
}

// Allocate will allocate some new IP for a specified pod
func (m *Manager) Allocate(networkName string, podInfo types.PodInfo, opts ...types.AllocateOption) (allocatedIPs []*types.IP, err error) {
	m.Lock()
	defer m.Unlock()

	var options = &types.AllocateOptions{}
	options.ApplyOptions(opts)

	validateFunctions := []func() error{
		func() error { return utils.CheckNotEmpty("network name", networkName) },
		func() error { return utils.CheckNotEmpty("pod name", podInfo.Name) },
		func() error { return utils.CheckNotEmpty("pod namespace", podInfo.Namespace) },
		func() error { return utils.CheckNotEmpty("ip family", string(podInfo.IPFamily)) },
	}

	if err = errors.AggregateGoroutines(validateFunctions...); err != nil {
		return nil, fmt.Errorf("validation fail: %v", err)
	}

	switch podInfo.IPFamily {
	case types.IPv4:
		return m.allocateIPv4(networkName, podInfo, *options)
	case types.IPv6:
		return m.allocateIPv6(networkName, podInfo, *options)
	case types.DualStack:
		return m.allocateDualStack(networkName, podInfo, *options)
	default:
		return nil, fmt.Errorf("unsupported ip family %s", podInfo.IPFamily)
	}
}

func (m *Manager) allocateIPv4(networkName string, podInfo types.PodInfo, options types.AllocateOptions) (IPs []*types.IP, err error) {
	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	var specifiedSubnetName string
	switch len(options.Subnets) {
	case 0:
		// empty specified subnet name means picking the first available one
		specifiedSubnetName = ""
	case 1:
		specifiedSubnetName = options.Subnets[0]
	default:
		return nil, fmt.Errorf("only support one specified subnet when IPv4 family, but %v", options.Subnets)
	}

	var subnet *types.Subnet
	if subnet, err = network.GetIPv4Subnet(specifiedSubnetName); err != nil {
		return nil, fmt.Errorf("fail to get ipv4 subnet: %v", err)
	}

	var ip *types.IP
	if ip = subnet.AllocateNext(podInfo.Name, podInfo.Namespace); ip == nil {
		return nil, fmt.Errorf("fail to get one available ipv4 address from subnet %s", subnet.Name)
	}

	IPs = append(IPs, ip)
	return
}

func (m *Manager) allocateIPv6(networkName string, podInfo types.PodInfo, options types.AllocateOptions) (IPs []*types.IP, err error) {
	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	var specifiedSubnetName string
	switch len(options.Subnets) {
	case 0:
		// empty specified subnet name means picking the first available one
		specifiedSubnetName = ""
	case 1:
		specifiedSubnetName = options.Subnets[0]
	default:
		return nil, fmt.Errorf("only support one specified subnet when IPv6 family, but %v", options.Subnets)
	}

	var subnet *types.Subnet
	if subnet, err = network.GetIPv6Subnet(specifiedSubnetName); err != nil {
		return nil, fmt.Errorf("fail to get ipv6 subnet: %v", err)
	}

	var ip *types.IP
	if ip = subnet.AllocateNext(podInfo.Name, podInfo.Namespace); ip == nil {
		return nil, fmt.Errorf("fail to get one available ipv6 address from subnet %s", subnet.Name)
	}

	IPs = append(IPs, ip)
	return
}

func (m *Manager) allocateDualStack(networkName string, podInfo types.PodInfo, options types.AllocateOptions) (IPs []*types.IP, err error) {
	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	var specifiedIPv4SubnetName, specifiedIPv6SubnetName string
	switch len(options.Subnets) {
	case 0:
		// empty specified subnet names mean picking the first available ones
		specifiedIPv4SubnetName, specifiedIPv6SubnetName = "", ""
	case 2:
		specifiedIPv4SubnetName, specifiedIPv6SubnetName = options.Subnets[0], options.Subnets[1]
	default:
		return nil, fmt.Errorf("only support two assigned subnets when DualStack family, but %v", options.Subnets)
	}

	var ipv4Subnet, ipv6Subnet *types.Subnet
	if ipv4Subnet, ipv6Subnet, err = network.GetDualStackSubnets(specifiedIPv4SubnetName, specifiedIPv6SubnetName); err != nil {
		return nil, fmt.Errorf("fail to get paired subnets: %v", err)
	}

	var ipv4IP, ipv6IP *types.IP
	if ipv4IP = ipv4Subnet.AllocateNext(podInfo.Name, podInfo.Namespace); ipv4IP == nil {
		return nil, fmt.Errorf("fail to get ipv4 address from subnet %s", ipv4Subnet.Name)
	}
	if ipv6IP = ipv6Subnet.AllocateNext(podInfo.Name, podInfo.Namespace); ipv6IP == nil {
		// recycle IPv4 address if IPv6 allocation fails
		ipv4Subnet.Release(ipv4IP.Address.IP.String())
		return nil, fmt.Errorf("fail to get ipv6 address zfrom subnet %s", ipv6Subnet.Name)
	}

	IPs = append(IPs, ipv4IP, ipv6IP)
	return
}

// Assign will recouple a specified pod with some allocated IPs
func (m *Manager) Assign(networkName string, podInfo types.PodInfo, assignedSuites []types.SubnetIPSuite, opts ...types.AssignOption) (assignedIPs []*types.IP, err error) {
	m.Lock()
	defer m.Unlock()

	var options = &types.AssignOptions{}
	options.ApplyOptions(opts)

	validateFunctions := []func() error{
		func() error { return utils.CheckNotEmpty("network name", networkName) },
		func() error { return utils.CheckNotEmpty("pod name", podInfo.Name) },
		func() error { return utils.CheckNotEmpty("pod namespace", podInfo.Namespace) },
		func() error { return utils.CheckNotEmpty("ip family", string(podInfo.IPFamily)) },
	}

	if err = errors.AggregateGoroutines(validateFunctions...); err != nil {
		return nil, fmt.Errorf("validation fail: %v", err)
	}

	switch podInfo.IPFamily {
	case types.IPv4:
		return m.assignIPv4(networkName, podInfo, assignedSuites, *options)
	case types.IPv6:
		return m.assignIPv6(networkName, podInfo, assignedSuites, *options)
	case types.DualStack:
		return m.assignDualStack(networkName, podInfo, assignedSuites, *options)
	default:
		return nil, fmt.Errorf("unsupported ip family %s", podInfo.IPFamily)
	}
}

func (m *Manager) assignIPv4(networkName string, podInfo types.PodInfo, assignedSuites []types.SubnetIPSuite, options types.AssignOptions) (assignedIPs []*types.IP, err error) {
	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	if len(assignedSuites) != 1 {
		return nil, fmt.Errorf("must assign only one IP when IPv4 family, but %v", assignedSuites)
	}

	var subnetName, ip = assignedSuites[0].Subnet, assignedSuites[0].IP
	if err = utils.ValidateIPv4(ip); err != nil {
		return
	}

	var subnet *types.Subnet
	if subnet, err = network.GetSubnetByNameOrIP(subnetName, ip); err != nil {
		return nil, fmt.Errorf("fail to get subnet by %v: %v", assignedSuites[0], err)
	}

	var assignedIP *types.IP
	if assignedIP, err = subnet.Assign(podInfo.Name, podInfo.Namespace, ip, options.Force); err != nil {
		return nil, fmt.Errorf("fail to assign ip %v to pod %s: %v", assignedSuites[0], podInfo, err)
	}

	assignedIPs = append(assignedIPs, assignedIP)
	return

}

func (m *Manager) assignIPv6(networkName string, podInfo types.PodInfo, assignedSuites []types.SubnetIPSuite, options types.AssignOptions) (assignedIPs []*types.IP, err error) {
	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	if len(assignedSuites) != 1 {
		return nil, fmt.Errorf("must assign only one IP when IPv6 family, but %v", assignedSuites)
	}

	var subnetName, ip = assignedSuites[0].Subnet, assignedSuites[0].IP
	if err = utils.ValidateIPv6(ip); err != nil {
		return
	}

	var subnet *types.Subnet
	if subnet, err = network.GetSubnetByNameOrIP(subnetName, ip); err != nil {
		return nil, fmt.Errorf("fail to get subnet by %v: %v", assignedSuites[0], err)
	}

	var assignedIP *types.IP
	if assignedIP, err = subnet.Assign(podInfo.Name, podInfo.Namespace, ip, options.Force); err != nil {
		return nil, fmt.Errorf("fail to assign ip %v to pod %s: %v", assignedSuites[0], podInfo, err)
	}

	assignedIPs = append(assignedIPs, assignedIP)
	return

}

func (m *Manager) assignDualStack(networkName string, podInfo types.PodInfo, assignedSuites []types.SubnetIPSuite, options types.AssignOptions) (assignedIPs []*types.IP, err error) {
	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	if len(assignedSuites) != 2 {
		return nil, fmt.Errorf("must assign two IPs when DualStack family, but %v", assignedSuites)
	}

	if err = utils.ValidateIPv4(assignedSuites[0].IP); err != nil {
		return
	}
	if err = utils.ValidateIPv6(assignedSuites[1].IP); err != nil {
		return
	}

	var v4Subnet, v6Subnet *types.Subnet
	if v4Subnet, err = network.GetSubnetByNameOrIP(assignedSuites[0].Subnet, assignedSuites[0].IP); err != nil {
		return nil, fmt.Errorf("fail to get subnet by %v: %v", assignedSuites[0], err)
	}
	if v6Subnet, err = network.GetSubnetByNameOrIP(assignedSuites[1].Subnet, assignedSuites[1].IP); err != nil {
		return nil, fmt.Errorf("fail to get subnet by %v: %v", assignedSuites[1], err)
	}

	var assignedIPv4, assignedIPv6 *types.IP
	if assignedIPv4, err = v4Subnet.Assign(podInfo.Name, podInfo.Namespace, assignedSuites[0].IP, options.Force); err != nil {
		return nil, fmt.Errorf("fail to assign ip %v to pod %s: %v", assignedSuites[0], podInfo, err)
	}
	if assignedIPv6, err = v6Subnet.Assign(podInfo.Name, podInfo.Namespace, assignedSuites[1].IP, options.Force); err != nil {
		return nil, fmt.Errorf("fail to assign ip %v to pod %s: %v", assignedSuites[1], podInfo, err)
	}

	assignedIPs = append(assignedIPs, assignedIPv4, assignedIPv6)
	return
}

// Release will release some IPs from allocation or assignment to a specified pod
func (m *Manager) Release(networkName string, releaseSuites []types.SubnetIPSuite) (err error) {
	m.Lock()
	defer m.Unlock()

	validateFunctions := []func() error{
		func() error { return utils.CheckNotEmpty("network name", networkName) },
	}
	if err = errors.AggregateGoroutines(validateFunctions...); err != nil {
		return fmt.Errorf("validation fail: %v", err)
	}

	var network *types.Network
	if network, err = m.Networks.GetNetwork(networkName); err != nil {
		return fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	for _, releaseSuite := range releaseSuites {
		if len(releaseSuite.Subnet) == 0 {
			return fmt.Errorf("must assign subnet when releasing IP, but %v", releaseSuite)
		}

		var subnet *types.Subnet
		if subnet, err = network.GetSubnet(releaseSuite.Subnet); err != nil {
			return
		}

		subnet.Release(releaseSuite.IP)
	}

	return
}

func (m *Manager) refreshNetwork(name string) error {
	// get network spec
	network, err := m.NetworkGetter(name)
	if err != nil {
		return err
	}

	if network == nil {
		m.Networks.RemoveNetwork(name)
		return nil
	}

	// get subnets which belongs to this network
	subnets, err := m.SubnetGetter(name)
	if err != nil {
		return err
	}

	var ips types.IPSet
	for _, subnet := range subnets {
		// get using ips which belongs to this subnet
		ips, err = m.IPSetGetter(subnet.Name)
		if err != nil {
			return err
		}
		if err = network.AddSubnet(subnet, ips); err != nil {
			return err
		}
	}

	m.Networks.RefreshNetwork(name, network)

	return nil
}
