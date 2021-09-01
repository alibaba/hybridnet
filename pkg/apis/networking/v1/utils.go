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

package v1

import (
	"fmt"
	"math"
	"math/big"
	"net"
)

func ValidateAddressRange(ar *AddressRange) (err error) {
	var (
		isIPv6  bool
		start   net.IP
		end     net.IP
		gateway net.IP
		cidr    *net.IPNet
		tempIP  net.IP
	)
	switch ar.Version {
	case IPv4:
		isIPv6 = false
	case IPv6:
		isIPv6 = true
	default:
		return fmt.Errorf("unsupported IP Version %s", ar.Version)
	}
	if start = net.ParseIP(ar.Start); len(ar.Start) > 0 && start == nil {
		return fmt.Errorf("invalid range start %s", ar.Start)
	}
	if end = net.ParseIP(ar.End); len(ar.End) > 0 && end == nil {
		return fmt.Errorf("invalid range end %s", ar.End)
	}
	if gateway = net.ParseIP(ar.Gateway); gateway == nil {
		return fmt.Errorf("invalid range gateway %s", ar.Gateway)
	}
	if gatewayIsIPv6 := gateway.To4() == nil; gatewayIsIPv6 != isIPv6 {
		return fmt.Errorf("address families of ip version and gateway mismatch")
	}
	if _, cidr, err = net.ParseCIDR(ar.CIDR); err != nil {
		return fmt.Errorf("invalid range CIDR %s", ar.CIDR)
	}

	if len(ar.Start) > 0 && !cidr.Contains(start) {
		return fmt.Errorf("start %s is not in CIDR %s", ar.Start, ar.CIDR)
	}
	if len(ar.End) > 0 && !cidr.Contains(end) {
		return fmt.Errorf("end %s is not in CIDR %s", ar.End, ar.CIDR)
	}
	if !cidr.Contains(gateway) {
		return fmt.Errorf("gateway %s is not in CIDR %s", ar.Gateway, ar.CIDR)
	}

	for _, rip := range ar.ReservedIPs {
		if tempIP = net.ParseIP(rip); tempIP == nil {
			return fmt.Errorf("invalid reserved ip %s", rip)
		} else if !cidr.Contains(tempIP) {
			return fmt.Errorf("reserved ip %s is not in CIDR %s", rip, ar.CIDR)
		}
	}

	for _, eip := range ar.ExcludeIPs {
		if tempIP = net.ParseIP(eip); tempIP == nil {
			return fmt.Errorf("invalid excluded ip %s", eip)
		} else if !cidr.Contains(tempIP) {
			return fmt.Errorf("excluded ip %s is not in CIDR %s", eip, ar.CIDR)
		}
	}

	return nil
}

func IsPrivateSubnet(subnetSpec *SubnetSpec) bool {
	if subnetSpec == nil || subnetSpec.Config == nil || subnetSpec.Config.Private == nil {
		return false
	}

	return *subnetSpec.Config.Private
}

func IsIPv6Subnet(subnetSpec *SubnetSpec) bool {
	if subnetSpec.Range.Version == IPv6 {
		return true
	}

	if gateway := net.ParseIP(subnetSpec.Range.Gateway); gateway != nil {
		return gateway.To4() == nil
	}

	return false
}

func IsSubnetAutoNatOutgoing(subnetSpec *SubnetSpec) bool {
	if subnetSpec == nil || subnetSpec.Config == nil || subnetSpec.Config.AutoNatOutgoing == nil {
		return true
	}

	return *subnetSpec.Config.AutoNatOutgoing
}

func CalculateCapacity(ar *AddressRange) int64 {
	var (
		cidr       *net.IPNet
		start, end net.IP
		err        error
	)

	if _, cidr, err = net.ParseCIDR(ar.CIDR); err != nil {
		return math.MaxInt64
	}

	if len(ar.Start) > 0 {
		start = net.ParseIP(ar.Start)
	}
	if start == nil {
		start = nextIP(cidr.IP)
	}

	if len(ar.End) > 0 {
		end = net.ParseIP(ar.End)
	}
	if end == nil {
		end = lastIP(cidr)
	}

	return capacity(start, end) - int64(len(ar.ExcludeIPs))
}

func GetNetworkType(networkObj *Network) NetworkType {
	if networkObj == nil || len(networkObj.Spec.Type) == 0 {
		return NetworkTypeUnderlay
	}

	return networkObj.Spec.Type
}

func GetRemoteSubnetType(remoteSubnetObj *RemoteSubnet) NetworkType {
	if remoteSubnetObj == nil || len(remoteSubnetObj.Spec.Type) == 0 {
		return NetworkTypeUnderlay
	}

	return remoteSubnetObj.Spec.Type
}

func lastIP(subnet *net.IPNet) net.IP {
	var end net.IP
	for i := 0; i < len(subnet.IP); i++ {
		end = append(end, subnet.IP[i]|^subnet.Mask[i])
	}
	if subnet.IP.To4() != nil {
		end[3]--
	}

	return end
}

func nextIP(ip net.IP) net.IP {
	i := ipToInt(ip)
	return intToIP(i.Add(i, big.NewInt(1)))
}

func capacity(a, b net.IP) int64 {
	aa := ipToInt(a)
	bb := ipToInt(b)
	return big.NewInt(0).Sub(bb, aa).Int64() + 1
}

func ipToInt(ip net.IP) *big.Int {
	if v := ip.To4(); v != nil {
		return big.NewInt(0).SetBytes(v)
	}
	return big.NewInt(0).SetBytes(ip.To16())
}

func intToIP(i *big.Int) net.IP {
	return net.IP(i.Bytes())
}
