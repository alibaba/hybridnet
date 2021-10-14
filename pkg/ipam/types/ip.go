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

import "fmt"

func NewIPSet() IPSet {
	return make(map[string]*IP)
}

func (s IPSet) Has(ip string) bool {
	_, found := s[ip]
	return found
}

func (s IPSet) Add(ip string, content *IP) {
	s[ip] = content
}

func (s IPSet) Update(ip, podName, podNamespace, status string) {
	if !s.Has(ip) {
		return
	}
	if len(podName) > 0 {
		s[ip].PodName = podName
	}
	if len(podNamespace) > 0 {
		s[ip].PodNamespace = podNamespace
	}
	if len(status) > 0 {
		s[ip].Status = status
	}
}

func (s IPSet) Delete(ip string) {
	delete(s, ip)
}

func (s IPSet) Get(ip string) *IP {
	return s[ip]
}

func (s IPSet) Count() int {
	return len(s)
}

func NewIPSlice() *IPSlice {
	return &IPSlice{
		IPs: make([]string, 0),
		// for the first allocation
		IPIndex: -1,
	}
}

func (s *IPSlice) Add(ip string, isDefault bool) {
	s.IPs = append(s.IPs, ip)
	s.IPCount = len(s.IPs)
	if isDefault {
		s.IPIndex = s.IPCount - 1
	}
}

func (s *IPSlice) Count() int {
	return s.IPCount
}

func (s *IPSlice) Next() string {
	s.IPIndex = (s.IPIndex + 1) % s.IPCount
	return s.IPs[s.IPIndex]
}

func (s *IPSlice) Current() string {
	if s.IPIndex < 0 {
		return ""
	}
	return s.IPs[s.IPIndex]
}

func (i *IP) String() string {
	return fmt.Sprintf("%s/%s/%s", i.Network, i.Subnet, i.Address.String())
}

func (i *IP) IsIPv6() bool {
	if i.Address == nil || i.Address.IP == nil {
		return false
	}
	return i.Address.IP.To4() == nil
}
