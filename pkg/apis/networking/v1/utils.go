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

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"

	"github.com/gogf/gf/container/gset"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/utils"
)

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

	if _, cidr, _ := net.ParseCIDR(subnet.Spec.Range.CIDR); cidr != nil {
		return cidr.IP.To4() == nil
	}
	return false
}

func GetNetworkType(networkObj *Network) NetworkType {
	if networkObj == nil || len(networkObj.Spec.Type) == 0 {
		return NetworkTypeUnderlay
	}

	return networkObj.Spec.Type
}

func GetNetworkMode(networkObj *Network) NetworkMode {
	switch GetNetworkType(networkObj) {
	case NetworkTypeUnderlay:
		if networkObj == nil || len(networkObj.Spec.Mode) == 0 {
			return NetworkModeVlan
		}
	case NetworkTypeOverlay:
		if len(networkObj.Spec.Mode) == 0 {
			return NetworkModeVxlan
		}
	case NetworkTypeGlobalBGP:
		if len(networkObj.Spec.Mode) == 0 {
			return NetworkModeGlobalBGP
		}
	}

	return networkObj.Spec.Mode
}

func IsGlobalUniqueNetwork(networkObj *Network) bool {
	return IsGlobalUniqueNetworkType(GetNetworkType(networkObj))
}

func IsGlobalUniqueNetworkType(networkType NetworkType) bool {
	return networkType == NetworkTypeOverlay || networkType == NetworkTypeGlobalBGP
}

func IsIPv6IPInstance(ip *IPInstance) bool {
	if ip == nil {
		return false
	}
	switch ip.Spec.Address.Version {
	case IPv4:
		return false
	case IPv6:
		return true
	default:
		if tempIP, _, _ := net.ParseCIDR(ip.Spec.Address.IP); tempIP != nil {
			return tempIP.To4() == nil
		}
	}
	return false
}

func ValidateAddressRange(ar *AddressRange) (err error) {
	var (
		isIPv6   bool
		start    net.IP
		end      net.IP
		gateway  net.IP
		ipOfCIDR net.IP
		cidr     *net.IPNet
		tempIP   net.IP
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

	if ipOfCIDR, cidr, err = net.ParseCIDR(ar.CIDR); err != nil {
		return fmt.Errorf("invalid range CIDR %s", ar.CIDR)
	}
	if !cidr.IP.Equal(ipOfCIDR) {
		return fmt.Errorf("CIDR notation is not standard, should start from %s but from %s", cidr.IP, ipOfCIDR)
	}
	ones, bits := cidr.Mask.Size()
	if ones == bits {
		return fmt.Errorf("types of /32 or /128 cidrs is not supported")
	}

	if len(ar.Start) > 0 && !cidr.Contains(start) {
		return fmt.Errorf("start %s is not in CIDR %s", ar.Start, ar.CIDR)
	}
	if len(ar.End) > 0 && !cidr.Contains(end) {
		return fmt.Errorf("end %s is not in CIDR %s", ar.End, ar.CIDR)
	}
	if len(ar.Start) > 0 && len(ar.End) > 0 && utils.Cmp(start, end) > 0 {
		return fmt.Errorf("subnet should have at least one available IP. start=%s, end=%s", start, end)
	}

	if len(ar.Gateway) != 0 {
		if gateway = net.ParseIP(ar.Gateway); gateway == nil {
			return fmt.Errorf("invalid range gateway %s", ar.Gateway)
		}
		if gatewayIsIPv6 := gateway.To4() == nil; gatewayIsIPv6 != isIPv6 {
			return fmt.Errorf("address families of ip version and gateway mismatch")
		}
		if !cidr.Contains(gateway) {
			return fmt.Errorf("gateway %s is not in CIDR %s", ar.Gateway, ar.CIDR)
		}
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
		start = utils.NextIP(cidr.IP)
	}

	if len(ar.End) > 0 {
		end = net.ParseIP(ar.End)
	}
	if end == nil {
		end = utils.LastIP(cidr)
	}

	return utils.Capacity(start, end) - int64(len(ar.ExcludeIPs))
}

func IsAvailable(statistics *Count) bool {
	if statistics == nil {
		return false
	}
	return statistics.Available > 0
}

// Intersect returns if ip address range of rangeA is overlapped with rangeB.
func Intersect(rangeA *AddressRange, rangeB *AddressRange) bool {
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
		startA = utils.NextIP(netA.IP)
	}
	if startB == nil {
		startB = utils.NextIP(netB.IP)
	}
	if endA == nil {
		endA = utils.LastIP(netA)
	}
	if endB == nil {
		endB = utils.LastIP(netB)
	}
	for i := startA; utils.Cmp(i, endA) <= 0; i = utils.NextIP(i) {
		if excludedIPSetA.Contains(i.String()) {
			continue
		}
		rangeASet.Add(i.String())
	}
	for i := startB; utils.Cmp(i, endB) <= 0; i = utils.NextIP(i) {
		if excludedIPSetB.Contains(i.String()) {
			continue
		}
		if rangeASet.Contains(i.String()) {
			return true
		}
	}
	return false
}

// IsLegacyModel will show whether IPInstance has switched to new version
// TODO: legacy mode, to be removed in the next major version
func IsLegacyModel(ipInstance *IPInstance) bool {
	return len(ipInstance.Spec.Binding.ReferredObject.Kind) == 0
}

func IsReserved(ipInstance *IPInstance) bool {
	if IsLegacyModel(ipInstance) {
		return len(ipInstance.Status.NodeName) == 0
	}

	return len(ipInstance.Spec.Binding.NodeName) == 0
}

func FetchBindingPodName(ipInstance *IPInstance) string {
	if IsLegacyModel(ipInstance) {
		return ipInstance.Labels[constants.LabelPod]
	}

	return ipInstance.Spec.Binding.PodName
}

func FetchBindingNodeName(ipInstance *IPInstance) string {
	if IsLegacyModel(ipInstance) {
		return ipInstance.Labels[constants.LabelNode]
	}

	return ipInstance.Spec.Binding.NodeName
}

func IsValidIPInstance(ipInstance *IPInstance) bool {
	if ipInstance == nil {
		return false
	}

	if IsLegacyModel(ipInstance) {
		return len(ipInstance.Status.Phase) > 0
	}

	return len(ipInstance.Spec.Binding.ReferredObject.Kind) > 0
}

func GetIndexFromName(name string) int {
	nameSlice := strings.Split(name, "-")
	indexStr := nameSlice[len(nameSlice)-1]

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return math.MaxInt32
	}
	return index
}
