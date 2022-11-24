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
	"fmt"
	"net"
	"strings"
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

func ValidateIP(ip string) error {
	if net.ParseIP(ip) != nil {
		return nil
	}
	return fmt.Errorf("%s is not a valid IP", ip)
}

func ValidateIPv4(ip string) error {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("%s is not a valid IP", ip)
	}
	if parsedIP.To4() == nil {
		return fmt.Errorf("%s is not an IPv4 address", ip)
	}
	return nil
}

func ValidateIPv6(ip string) error {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("%s is not a valid IP", ip)
	}
	if parsedIP.To4() != nil {
		return fmt.Errorf("%s is not an IPv6 address", ip)
	}
	return nil
}

func ToDNSFormat(ip net.IP) string {
	if ip.To4() == nil {
		return strings.ReplaceAll(unifyIPv6AddressString(ip.String()), ":", "-")
	}
	return strings.ReplaceAll(ip.String(), ".", "-")
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

// unifyIPv6AddressString will help to extend the squashed sections in IPv6 address string,
// eg, 234e:0:4567::5f will be unified to 234e:0:4567:0:0:0:0:5f
func unifyIPv6AddressString(ip string) string {
	const maxSectionCount = 8

	if sectionCount := strings.Count(ip, ":") + 1; sectionCount < maxSectionCount {
		var separators = []string{":", ":"}
		for ; sectionCount < maxSectionCount; sectionCount++ {
			separators = append(separators, ":")
		}
		return strings.ReplaceAll(ip, "::", strings.Join(separators, "0"))
	}

	return ip
}
