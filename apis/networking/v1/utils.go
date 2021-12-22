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

package v1

import "net"

// TODO: unit tests

func IsPrivateSubnet(subnet *Subnet) bool {
	if subnet == nil || subnet.Spec.Config == nil || subnet.Spec.Config.Private == nil {
		return false
	}

	return *subnet.Spec.Config.Private
}

func IsIPv6Subnet(subnet *Subnet) bool {
	if subnet == nil {
		return false
	}
	if subnet.Spec.Range.Version == IPv6 {
		return true
	}
	if gateway := net.ParseIP(subnet.Spec.Range.Gateway); gateway != nil {
		return gateway.To4() == nil
	}
	return false
}

func GetNetworkType(networkObj *Network) NetworkType {
	if networkObj == nil || len(networkObj.Spec.Type) == 0 {
		return NetworkTypeUnderlay
	}

	return networkObj.Spec.Type
}

func IsIPv6IPInstance(ip *IPInstance) bool {
	if ip == nil {
		return false
	}
	if ip.Spec.Address.Version == IPv6 {
		return true
	}
	if tempIP, _, _ := net.ParseCIDR(ip.Spec.Address.IP); tempIP != nil {
		return tempIP.To4() == nil
	}
	return false
}
