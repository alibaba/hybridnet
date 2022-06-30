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

	"github.com/containernetworking/plugins/pkg/ip"

	"github.com/alibaba/hybridnet/pkg/utils"
)

var (
	ErrNoAvailableSubnet      = errors.New("no available subnet")
	ErrNotFoundSubnet         = errors.New("subnet not found")
	ErrNotFoundAssignedIP     = errors.New("assigned ip not found")
	ErrNotAvailableAssignedIP = errors.New("assigned ip is not available")
)

func NewSubnetSlice() *SubnetSlice {
	return &SubnetSlice{
		Subnets:        make([]*Subnet, 0),
		SubnetIndexMap: make(map[string]int),
	}
}

func (s *SubnetSlice) AddSubnet(subnet *Subnet, parentNetID *uint32, ips IPSet, isDefault bool) error {
	if err := subnet.Canonicalize(); err != nil {
		return err
	}

	if err := subnet.Sync(parentNetID, ips); err != nil {
		return err
	}

	s.Subnets = append(s.Subnets, subnet)
	s.SubnetCount = len(s.Subnets)
	s.SubnetIndexMap[subnet.Name] = s.SubnetCount - 1
	if isDefault {
		s.SubnetIndex = s.SubnetIndexMap[subnet.Name]
	}

	return nil
}

func (s *SubnetSlice) GetSubnet(name string) (*Subnet, error) {
	if subnetIndex, exist := s.SubnetIndexMap[name]; exist {
		return s.Subnets[subnetIndex], nil
	}

	return nil, ErrNotFoundSubnet
}

func (s *SubnetSlice) GetAvailableSubnet() (*Subnet, error) {
	if s.SubnetCount == 0 {
		return nil, ErrNoAvailableSubnet
	}

	lastIndex := s.SubnetIndex
	for {
		if s.Subnets[s.SubnetIndex].IsAvailable() {
			return s.Subnets[s.SubnetIndex], nil
		}

		s.SubnetIndex = (s.SubnetIndex + 1) % s.SubnetCount
		if s.SubnetIndex == lastIndex {
			return nil, ErrNoAvailableSubnet
		}
	}
}

func (s *SubnetSlice) GetAvailableIPv4Subnet() (*Subnet, error) {
	if s.SubnetCount == 0 {
		return nil, ErrNoAvailableSubnet
	}

	lastIndex := s.SubnetIndex
	for {
		var subnet = s.Subnets[s.SubnetIndex]

		if subnet.IsIPv4() && subnet.IsAvailable() {
			return subnet, nil
		}

		s.SubnetIndex = (s.SubnetIndex + 1) % s.SubnetCount
		if s.SubnetIndex == lastIndex {
			return nil, ErrNoAvailableSubnet
		}
	}
}

func (s *SubnetSlice) GetAvailableIPv6Subnet() (*Subnet, error) {
	if s.SubnetCount == 0 {
		return nil, ErrNoAvailableSubnet
	}

	lastIndex := s.SubnetIndex
	for {
		var subnet = s.Subnets[s.SubnetIndex]

		if subnet.IsIPv6() && subnet.IsAvailable() {
			return subnet, nil
		}

		s.SubnetIndex = (s.SubnetIndex + 1) % s.SubnetCount
		if s.SubnetIndex == lastIndex {
			return nil, ErrNoAvailableSubnet
		}
	}
}

func (s *SubnetSlice) GetAvailableDualStackSubnets() (v4Subnet *Subnet, v6Subnet *Subnet, err error) {
	if s.SubnetCount == 0 {
		return nil, nil, ErrNoAvailableSubnet
	}

	lastIndex := s.SubnetIndex
	var v4Selected, v6Selected = false, false
	for {
		var subnet = s.Subnets[s.SubnetIndex]

		if subnet.IsAvailable() {
			if !v4Selected && subnet.IsIPv4() {
				v4Subnet = subnet
				v4Selected = true
			}
			if !v6Selected && subnet.IsIPv6() {
				v6Subnet = subnet
				v6Selected = true
			}
		}

		if v4Selected && v6Selected {
			return
		}

		s.SubnetIndex = (s.SubnetIndex + 1) % s.SubnetCount
		if s.SubnetIndex == lastIndex {
			return nil, nil, ErrNoAvailableSubnet
		}
	}
}

func (s *SubnetSlice) GetSubnetByIP(ip string) (*Subnet, error) {
	for _, subnet := range s.Subnets {
		if subnet.Contains(net.ParseIP(ip)) {
			return subnet, nil
		}
	}

	return nil, ErrNotFoundSubnet
}

func (s *SubnetSlice) CurrentSubnet() string {
	if s.SubnetCount == 0 {
		return ""
	}

	return s.Subnets[s.SubnetIndex].Name
}

func (s *SubnetSlice) Usage() (string, map[string]*Usage) {
	usages := make(map[string]*Usage, len(s.Subnets))

	for _, subnet := range s.Subnets {
		usages[subnet.Name] = subnet.Usage()
	}

	return s.CurrentSubnet(), usages
}

func (s *SubnetSlice) Classify() (IPv4Subnets, IPv6Subnets []string) {
	for _, subnet := range s.Subnets {
		if subnet.IsIPv6() {
			IPv6Subnets = append(IPv6Subnets, subnet.Name)
		} else {
			IPv4Subnets = append(IPv4Subnets, subnet.Name)
		}
	}
	return
}

func NewSubnet(
	name, network string, netID *uint32,
	start, end, gateway net.IP, cidr *net.IPNet,
	reservedList, blackList map[string]struct{}, lastAllocated net.IP,
	private, IPv6 bool) *Subnet {
	return &Subnet{
		Name:            name,
		ParentNetwork:   network,
		NetID:           netID,
		Start:           start,
		End:             end,
		CIDR:            cidr,
		Gateway:         gateway,
		ReservedList:    reservedList,
		BlackList:       blackList,
		LastAllocatedIP: lastAllocated,
		Private:         private,
		IPv6:            IPv6,
	}
}

// Canonicalize takes a given subnet and ensures that all information is consistent,
// filling out Start, End, and Gateway with sane values if missing
func (s *Subnet) Canonicalize() error {
	if err := s.Validate(); err != nil {
		return err
	}

	if s.Start == nil {
		s.Start = ip.NextIP(s.CIDR.IP)
	}

	if s.End == nil {
		s.End = utils.LastIP(s.CIDR)
	}

	return nil
}

// Validate can ensure that all necessary information are valid
func (s *Subnet) Validate() error {
	// Basic validations
	switch {
	case len(s.Name) == 0:
		return fmt.Errorf("subnet name can not be empty")
	case len(s.ParentNetwork) == 0:
		return fmt.Errorf("subnet's partent network can not be empty")
	case s.CIDR.IP == nil || s.CIDR.Mask == nil:
		return fmt.Errorf("CIDR is invalid")
	}

	// Can't create an allocator for a network with no addresses, eg a /32 or /31
	ones, masklen := s.CIDR.Mask.Size()
	if ones > masklen-2 {
		return fmt.Errorf("CIDR %s too small to allocate from", s.CIDR.String())
	}

	if len(s.CIDR.IP) != len(s.CIDR.Mask) {
		return fmt.Errorf("CIDR %s IPNet IP and Mask version mismatch", s.CIDR.String())
	}

	// Ensure Subnet IP is the network address, not some other address
	networkIP := s.CIDR.IP.Mask(s.CIDR.Mask)
	if !s.CIDR.IP.Equal(networkIP) {
		return fmt.Errorf("CIDR has host bits set because a subnet mask of length %d the network address is %s", ones, networkIP.String())
	}

	// Gateway must in CIDR
	if s.Gateway != nil && !s.CIDR.Contains(s.Gateway) {
		return fmt.Errorf("gateway %s not in CIDR %s", s.Gateway.String(), s.CIDR.String())
	}

	// Start must in CIDR
	if s.Start != nil {
		if !s.CIDR.Contains(s.Start) {
			return fmt.Errorf("start %s not in CIDR %s", s.Start.String(), s.CIDR.String())
		}
	}

	// End must in CIDR
	if s.End != nil {
		if !s.CIDR.Contains(s.End) {
			return fmt.Errorf("end %s not in CIDR %s", s.End.String(), s.CIDR.String())
		}
	}

	return nil
}

// Contains checks if a given ip is a valid, allocatable address in a given Range
// This address should be in CIDR [start,gw) (gw,end], and not in black list.
func (s *Subnet) Contains(addr net.IP) bool {
	if !s.CIDR.Contains(addr) {
		return false
	}

	// We ignore nils here so we can use this function as we initialize the range
	if s.Start != nil {
		if ip.Cmp(addr, s.Start) < 0 {
			return false
		}
	}

	if s.End != nil {
		if ip.Cmp(addr, s.End) > 0 {
			return false
		}
	}

	if s.Gateway != nil && s.Gateway.Equal(addr) {
		return false
	}

	if s.IsBlackIP(addr.String()) {
		return false
	}

	return true
}

// Sync will generate netID, filtered Reserved List, Available IP Slice
// and Using IP Set based on subnet spec and input
func (s *Subnet) Sync(parentNetID *uint32, ipSet IPSet) error {
	// generate valid netID, inherit from parent if NetID is null
	if s.NetID == nil {
		s.NetID = parentNetID
	}

	// filter reserved list
	filteredReservedList := make(map[string]struct{})
	for rip := range s.ReservedList {
		if s.Contains(net.ParseIP(rip)) {
			filteredReservedList[rip] = struct{}{}
		}
	}
	s.ReservedList = filteredReservedList
	s.ReservedIPCount = len(s.ReservedList)

	// generate valid Using IP Set
	s.UsingIPs = NewIPSet()
	for ip, content := range ipSet {
		if content.Subnet == s.Name && s.Contains(content.Address.IP) {
			s.UsingIPs.Add(ip, content)
		}
	}

	// pre-assign reserved ip
	for rip := range s.ReservedList {
		if !s.UsingIPs.Has(rip) {
			s.UsingIPs.Add(rip, &IP{
				Address: &net.IPNet{
					IP:   net.ParseIP(rip),
					Mask: s.CIDR.Mask,
				},
				Gateway:      s.Gateway,
				NetID:        s.NetID,
				Subnet:       s.Name,
				Network:      s.ParentNetwork,
				PodName:      "",
				PodNamespace: "",
				Status:       IPStatusReserved,
			})
		}
	}

	// generate valid Available IP Slice
	s.AvailableIPs = NewIPSlice()
	for i := s.Start; ip.Cmp(i, s.End) <= 0; i = ip.NextIP(i) {
		if !s.Contains(i) {
			continue
		}
		// ignore reserved ip
		if s.IsReservedIP(i.String()) {
			continue
		}
		s.AvailableIPs.Add(i.String(), i.Equal(s.LastAllocatedIP))
	}

	return nil
}

// Overlap must be called **after** Canonicalize
func (s *Subnet) Overlap(s1 *Subnet) bool {
	if s.IsIPv6() != s1.IsIPv6() {
		return false
	}

	return s.Contains(s1.Start) ||
		s.Contains(s1.End) ||
		s1.Contains(s.Start) ||
		s1.Contains(s.End)
}

func (s *Subnet) IsAvailable() bool {
	return s.AvailableIPs.Count() > s.UsingIPCount() && !s.Private
}

// UsingIPCount will count the IP which are being used, but
// the reserved IPs will be excluded
func (s *Subnet) UsingIPCount() int {
	return s.UsingIPs.Count() - s.ReservedIPCount
}

func (s *Subnet) Usage() *Usage {
	return &Usage{
		Total:          uint32(s.AvailableIPs.Count()),
		Used:           uint32(s.UsingIPCount()),
		Available:      uint32(s.AvailableIPs.Count() - s.UsingIPCount()),
		LastAllocation: s.AvailableIPs.Current(),
	}
}

func (s *Subnet) AllocateNext(podName, podNamespace string) *IP {
	for i := 0; i < s.AvailableIPs.Count(); i++ {
		ipCandidate := s.AvailableIPs.Next()
		if s.UsingIPs.Has(ipCandidate) {
			continue
		}

		availableIP := &IP{
			Address: &net.IPNet{
				IP:   net.ParseIP(ipCandidate),
				Mask: s.CIDR.Mask,
			},
			Gateway:      s.Gateway,
			NetID:        s.NetID,
			Subnet:       s.Name,
			Network:      s.ParentNetwork,
			PodName:      podName,
			PodNamespace: podNamespace,
			Status:       IPStatusAllocated,
		}

		s.UsingIPs.Add(ipCandidate, availableIP)

		return availableIP
	}

	return nil
}

func (s *Subnet) Release(ip string) {
	if s.IsReservedIP(ip) {
		s.UsingIPs.Update(ip, "", "", IPStatusReserved)
	} else {
		s.UsingIPs.Delete(ip)
	}
}

func (s *Subnet) Reserve(ip string) {
	s.UsingIPs.UpdateStatus(ip, IPStatusReserved)
}

func (s *Subnet) Assign(podName, podNamespace, ip string, forced bool) (*IP, error) {
	if !s.Contains(net.ParseIP(ip)) {
		return nil, ErrNotFoundAssignedIP
	}

	switch {
	case !s.UsingIPs.Has(ip):
		s.UsingIPs.Add(ip, &IP{
			Address: &net.IPNet{
				IP:   net.ParseIP(ip),
				Mask: s.CIDR.Mask,
			},
			Gateway:      s.Gateway,
			NetID:        s.NetID,
			Subnet:       s.Name,
			Network:      s.ParentNetwork,
			PodName:      podName,
			PodNamespace: podNamespace,
			Status:       IPStatusAllocated,
		})
	case s.UsingIPs.Get(ip).PodNamespace == podNamespace && s.UsingIPs.Get(ip).PodName == podName:
		s.UsingIPs.Update(ip, podName, podNamespace, IPStatusAllocated)
	case forced && s.UsingIPs.Get(ip).Status == IPStatusReserved:
		s.UsingIPs.Update(ip, podName, podNamespace, IPStatusAllocated)
	default:
		return nil, ErrNotAvailableAssignedIP
	}

	return s.UsingIPs.Get(ip), nil
}

func (s *Subnet) IsReservedIP(ip string) bool {
	_, found := s.ReservedList[ip]
	return found
}

func (s *Subnet) IsBlackIP(ip string) bool {
	_, found := s.BlackList[ip]
	return found
}

func (s *Subnet) IsIPv6() bool {
	return s.IPv6
}

func (s *Subnet) IsIPv4() bool {
	return !s.IPv6
}
