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

type DualStackAllocator struct {
	sync.RWMutex

	Networks types.NetworkSet

	NetworkGetter NetworkGetter
	SubnetGetter  SubnetGetter
	IPSetGetter   IPSetGetter
}

func NewDualStackAllocator(networks []string, nGetter NetworkGetter, sGetter SubnetGetter, iGetter IPSetGetter) (*DualStackAllocator, error) {
	allocator := &DualStackAllocator{
		RWMutex:       sync.RWMutex{},
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

func (d *DualStackAllocator) Refresh(networks []string) error {
	d.Lock()
	defer d.Unlock()

	for _, network := range networks {
		if err := d.refreshNetwork(network); err != nil {
			return err
		}
	}

	return nil
}

func (d *DualStackAllocator) Usage(networkName string) ([3]*types.Usage, map[string]*types.Usage, error) {
	d.RLock()
	defer d.RUnlock()

	network, err := d.Networks.GetNetwork(networkName)
	if err != nil {
		return [3]*types.Usage{}, nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	return network.DualStackUsage()
}

func (d *DualStackAllocator) SubnetUsage(networkName, subnetName string) (*types.Usage, error) {
	d.RLock()
	defer d.RUnlock()

	network, err := d.Networks.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	subnet, err := network.GetSubnet(subnetName)
	if err != nil {
		return nil, fmt.Errorf("fail to get subnet %s: %v", subnetName, err)
	}

	return subnet.Usage(), nil
}

func (d *DualStackAllocator) Allocate(ipFamilyMode types.IPFamilyMode, network string, subnets []string, podName, podNamespace string) (IPs []*types.IP, err error) {
	d.Lock()
	defer d.Unlock()

	switch ipFamilyMode {
	case types.IPv4Only:
		return d.allocateIPv4Only(network, subnets, podName, podNamespace)
	case types.IPv6Only:
		return d.allocateIPv6Only(network, subnets, podName, podNamespace)
	case types.DualStack:
		return d.allocateDualStack(network, subnets, podName, podNamespace)
	default:
		return nil, fmt.Errorf("unsupported ip family %s", ipFamilyMode)
	}
}

func (d *DualStackAllocator) allocateIPv4Only(networkName string, subnets []string, podName, podNamespace string) (IPs []*types.IP, err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return
	}

	var subnetName string
	switch len(subnets) {
	case 0:
		subnetName = ""
	case 1:
		subnetName = subnets[0]
	default:
		return nil, fmt.Errorf("only support one assigned subnet when IPv4Only mode")
	}

	var subnet *types.Subnet
	if subnet, err = network.GetIPv4Subnet(subnetName); err != nil {
		return nil, fmt.Errorf("fail to get ipv4 subnet: %v", err)
	}

	var ipv4Candidate *types.IP
	if ipv4Candidate = subnet.AllocateNext(podName, podNamespace); ipv4Candidate == nil {
		return nil, fmt.Errorf("fail to get available ipv4 from subnet %s", subnet.Name)
	}

	IPs = append(IPs, ipv4Candidate)
	return
}

func (d *DualStackAllocator) allocateIPv6Only(networkName string, subnets []string, podName, podNamespace string) (IPs []*types.IP, err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return
	}

	var subnetName string
	switch len(subnets) {
	case 0:
		subnetName = ""
	case 1:
		subnetName = subnets[0]
	default:
		return nil, fmt.Errorf("only support one assigned subnet when IPv6Only mode")
	}

	var subnet *types.Subnet
	if subnet, err = network.GetIPv6Subnet(subnetName); err != nil {
		return nil, fmt.Errorf("fail to get ipv6 subnet: %v", err)
	}

	var ipv6Candidate *types.IP
	if ipv6Candidate = subnet.AllocateNext(podName, podNamespace); ipv6Candidate == nil {
		return nil, fmt.Errorf("fail to get available ipv6 from subnet %s", subnet.Name)
	}

	IPs = append(IPs, ipv6Candidate)
	return
}

func (d *DualStackAllocator) allocateDualStack(networkName string, subnets []string, podName, podNamespace string) (IPs []*types.IP, err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return
	}

	var v4Name, v6Name string
	switch len(subnets) {
	case 0:
		v4Name, v6Name = "", ""
	case 2:
		v4Name, v6Name = subnets[0], subnets[1]
	default:
		return nil, fmt.Errorf("only support 2 assigned subnets when DualStack mode")
	}

	var v4Subnet, v6Subnet *types.Subnet
	if v4Subnet, v6Subnet, err = network.GetPairedDualStackSubnets(v4Name, v6Name); err != nil {
		return nil, fmt.Errorf("fail to get paired subnets: %v", err)
	}

	var ipv4Candidate, ipv6Candidate *types.IP
	if ipv4Candidate = v4Subnet.AllocateNext(podName, podNamespace); ipv4Candidate == nil {
		return nil, fmt.Errorf("fail to get paired ipv4 from subnet %s", v4Subnet.Name)
	}
	if ipv6Candidate = v6Subnet.AllocateNext(podName, podNamespace); ipv6Candidate == nil {
		// recycle IPv4 address if IPv6 allocation fails
		v4Subnet.Release(ipv4Candidate.Address.IP.String())
		return nil, fmt.Errorf("fail to get paired ipv6 from subnet %s", v6Subnet.Name)
	}

	IPs = append(IPs, ipv4Candidate, ipv6Candidate)
	return
}

func (d *DualStackAllocator) Assign(ipFamilyMode types.IPFamilyMode, network string, subnets, IPs []string,
	podName, podNamespace string, forced bool) (assignedIPs []*types.IP, err error) {
	d.Lock()
	defer d.Unlock()

	switch ipFamilyMode {
	case types.IPv4Only:
		return d.assignIPv4Only(network, subnets, IPs, podName, podNamespace, forced)
	case types.IPv6Only:
		return d.assignIPv6Only(network, subnets, IPs, podName, podNamespace, forced)
	case types.DualStack:
		return d.assignDualStack(network, subnets, IPs, podName, podNamespace, forced)
	default:
		return nil, fmt.Errorf("unsupported ip family %s", ipFamilyMode)
	}
}

func (d *DualStackAllocator) assignIPv4Only(networkName string, subnets, IPs []string, podName, podNamespace string, forced bool) (assignedIPs []*types.IP, err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return
	}

	var subnetName string
	switch len(subnets) {
	case 0:
		subnetName = ""
	case 1:
		subnetName = subnets[0]
	default:
		return nil, fmt.Errorf("only support one assigned subnet when IPv4Only mode")
	}

	var ip string
	if len(IPs) != 1 {
		return nil, fmt.Errorf("only support one assigned IP when IPv4Only mode")
	}
	if len(IPs[0]) == 0 {
		return nil, fmt.Errorf("invalid assigned IP %s", IPs[0])
	}
	ip = IPs[0]

	var subnet *types.Subnet
	if subnet, err = network.GetSubnetByIP(subnetName, ip); err != nil {
		return nil, fmt.Errorf("fail to get subnets %s by ip %s: %v", subnetName, ip, err)
	}

	var assignedIP *types.IP
	if assignedIP, err = subnet.Assign(podName, podNamespace, ip, forced); err != nil {
		return nil, fmt.Errorf("fail to assign ip %s in subnet %s: %v", ip, subnetName, err)
	}

	assignedIPs = append(assignedIPs, assignedIP)
	return
}

func (d *DualStackAllocator) assignIPv6Only(networkName string, subnets, IPs []string, podName, podNamespace string, forced bool) (assignedIPs []*types.IP, err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return
	}

	var subnetName string
	switch len(subnets) {
	case 0:
		subnetName = ""
	case 1:
		subnetName = subnets[0]
	default:
		return nil, fmt.Errorf("only support one assigned subnet when IPv6Only mode")
	}

	var ip string
	if len(IPs) != 1 {
		return nil, fmt.Errorf("only support one assigned IP when IPv6Only mode")
	}
	if len(IPs[0]) == 0 {
		return nil, fmt.Errorf("invalid assigned IP %s", IPs[0])
	}
	ip = IPs[0]

	var subnet *types.Subnet
	if subnet, err = network.GetSubnetByIP(subnetName, ip); err != nil {
		return nil, fmt.Errorf("fail to get subnets %s by ip %s: %v", subnetName, ip, err)
	}

	var assignedIP *types.IP
	if assignedIP, err = subnet.Assign(podName, podNamespace, ip, forced); err != nil {
		return nil, fmt.Errorf("fail to assign ip %s in subnet %s: %v", ip, subnetName, err)
	}

	assignedIPs = append(assignedIPs, assignedIP)
	return
}

func (d *DualStackAllocator) assignDualStack(networkName string, subnets, IPs []string, podName, podNamespace string, forced bool) (assignedIPs []*types.IP, err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return
	}

	var v4Name, v6Name string
	switch len(subnets) {
	case 0:
		v4Name, v6Name = "", ""
	case 2:
		v4Name, v6Name = subnets[0], subnets[1]
	default:
		return nil, fmt.Errorf("only support 2 assigned subnets when DualStack mode")
	}

	var ipv4, ipv6 string
	if len(IPs) != 2 {
		return nil, fmt.Errorf("only support 2 assigned IPs when DualStack mode")
	}
	if len(IPs[0]) == 0 || len(IPs[1]) == 0 {
		return nil, fmt.Errorf("invalid assigned IP %s or %s", IPs[0], IPs[1])
	}
	ipv4, ipv6 = IPs[0], IPs[1]

	var v4Subnet, v6Subnet *types.Subnet
	if v4Subnet, err = network.GetSubnetByIP(v4Name, ipv4); err != nil {
		return nil, fmt.Errorf("fail to get subnet %s by ip %s: %v", v4Name, ipv4, err)
	}
	if v6Subnet, err = network.GetSubnetByIP(v6Name, ipv6); err != nil {
		return nil, fmt.Errorf("fail to get subnet %s by ip %s: %v", v6Name, ipv6, err)
	}

	var assignedIPv4, assignedIPv6 *types.IP
	if assignedIPv4, err = v4Subnet.Assign(podName, podNamespace, ipv4, forced); err != nil {
		return nil, fmt.Errorf("fail to assign ip %s in subnet %s: %v", ipv4, v4Name, err)
	}
	if assignedIPv6, err = v6Subnet.Assign(podName, podNamespace, ipv6, forced); err != nil {
		return nil, fmt.Errorf("fail to assign ip %s in subnet %s: %v", ipv6, v6Name, err)
	}

	assignedIPs = append(assignedIPs, assignedIPv4, assignedIPv6)
	return
}

func (d *DualStackAllocator) Release(ipFamilyMode types.IPFamilyMode, networkName string, subnets, IPs []string) (err error) {
	d.Lock()
	defer d.Unlock()

	switch ipFamilyMode {
	case types.IPv4Only, types.IPv6Only:
		return d.releaseIP(networkName, subnets, IPs)
	case types.DualStack:
		return d.releaseIPs(networkName, subnets, IPs)
	default:
		return fmt.Errorf("unsupported ip family %s", ipFamilyMode)
	}
}

func (d *DualStackAllocator) releaseIP(networkName string, subnets, IPs []string) (err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	if len(subnets) != 1 {
		return fmt.Errorf("subnets has unexpected length %d", len(subnets))
	}
	if len(IPs) != 1 {
		return fmt.Errorf("IPs has unexpected length %d", len(IPs))
	}

	var subnet *types.Subnet
	if subnet, err = network.GetSubnet(subnets[0]); err != nil {
		return fmt.Errorf("fail to get subnet %s: %v", subnets[0], err)
	}

	subnet.Release(IPs[0])
	return nil
}

func (d *DualStackAllocator) releaseIPs(networkName string, subnets, IPs []string) (err error) {
	var network *types.Network
	if network, err = d.Networks.GetNetwork(networkName); err != nil {
		return fmt.Errorf("fail to get network %s: %v", networkName, err)
	}

	if len(subnets) != len(IPs) {
		return fmt.Errorf("subnets mismatch IPs in length %d/%d", len(subnets), len(IPs))
	}

	for i := range subnets {
		var subnet *types.Subnet
		if subnet, err = network.GetSubnet(subnets[i]); err != nil {
			return fmt.Errorf("fail to get subnet %s: %v", subnets[i], err)
		}

		subnet.Release(IPs[i])
	}

	return nil
}

func (d *DualStackAllocator) refreshNetwork(name string) error {
	// get network spec
	network, err := d.NetworkGetter(name)
	if err != nil {
		return err
	}

	if network == nil {
		d.Networks.RemoveNetwork(name)
		return nil
	}

	// get subnets which belongs to this network
	subnets, err := d.SubnetGetter(name)
	if err != nil {
		return err
	}

	var ips types.IPSet
	for _, subnet := range subnets {
		// get using ips which belongs to this subnet
		ips, err = d.IPSetGetter(subnet.Name)
		if err != nil {
			return err
		}
		if err = network.AddSubnet(subnet, ips); err != nil {
			return err
		}
	}

	d.Networks.RefreshNetwork(name, network)

	return nil
}

func (d *DualStackAllocator) GetNetworksByType(networkType types.NetworkType) []string {
	d.RLock()
	defer d.RUnlock()

	return d.Networks.GetNetworksByType(networkType)
}

func (d *DualStackAllocator) MatchNetworkType(networkName string, networkType types.NetworkType) bool {
	d.RLock()
	defer d.RUnlock()

	return d.Networks.MatchNetworkType(networkName, networkType)
}
