/*
  Copyright 2021 The Rama Authors.

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

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
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

func Intersect(cidr1 string, ipVersion1 networkingv1.IPVersion,
	cidr2 string, ipVersion2 networkingv1.IPVersion) bool {
	if ipVersion1 != ipVersion2 {
		return false
	}
	_, net1, _ := net.ParseCIDR(cidr1)
	_, net2, _ := net.ParseCIDR(cidr2)

	return net1.Contains(net2.IP) || net2.Contains(net1.IP)
}
