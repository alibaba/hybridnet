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

package utils

import (
	"net"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/gogf/gf/container/gset"

	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
)

func StringToIPNet(in string) *net.IPNet {
	ip, cidr, _ := net.ParseCIDR(in)
	cidr.IP = ip
	return cidr
}

// NormalizedIP If IP is valid, return itself otherwise empty string
func NormalizedIP(ip string) string {
	if net.ParseIP(ip) != nil {
		return ip
	}
	return ""
}

// Intersect returns if ip address range of rangeA is overlapped with rangeB.
func Intersect(rangeA *networkingv1.AddressRange, rangeB *networkingv1.AddressRange) bool {
	if rangeA.Version != rangeB.Version {
		return false
	}
	var (
		netA *net.IPNet
		netB *net.IPNet
	)

	_, netA, _ = net.ParseCIDR(rangeA.CIDR)
	_, netB, _ = net.ParseCIDR(rangeB.CIDR)

	if !netA.Contains(netB.IP) && !netB.Contains(netA.IP) {
		return false
	}

	var (
		startA         = net.ParseIP(rangeA.Start)
		endA           = net.ParseIP(rangeA.End)
		excludedIPSetA = gset.NewStrSetFrom(rangeA.ExcludeIPs)
		startB         = net.ParseIP(rangeB.Start)
		endB           = net.ParseIP(rangeB.End)
		excludedIPSetB = gset.NewStrSetFrom(rangeB.ExcludeIPs)
		rangeASet      = gset.NewStrSet()
	)
	if startA == nil {
		startA = ip.NextIP(netA.IP)
	}
	if startB == nil {
		startB = ip.NextIP(netB.IP)
	}
	if endA == nil {
		endA = LastIP(netA)
	}
	if endB == nil {
		endB = LastIP(netB)
	}
	for i := startA; ip.Cmp(i, endA) <= 0; i = ip.NextIP(i) {
		if excludedIPSetA.Contains(i.String()) {
			continue
		}
		rangeASet.Add(i.String())
	}
	for i := startB; ip.Cmp(i, endB) <= 0; i = ip.NextIP(i) {
		if excludedIPSetB.Contains(i.String()) {
			continue
		}
		if rangeASet.Contains(i.String()) {
			return true
		}
	}
	return false
}

// LastIP Determine the last IP of a subnet, excluding the broadcast if IPv4
func LastIP(subnet *net.IPNet) net.IP {
	var end net.IP
	for i := 0; i < len(subnet.IP); i++ {
		end = append(end, subnet.IP[i]|^subnet.Mask[i])
	}
	if subnet.IP.To4() != nil {
		end[3]--
	}

	return end
}
