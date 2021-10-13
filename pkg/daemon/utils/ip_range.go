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
	"math/big"
	"net"
	"sort"

	"github.com/containernetworking/plugins/pkg/ip"
)

type IPRange struct {
	start net.IP
	end   net.IP
}

func CreateIPRange(start, end net.IP) (*IPRange, error) {
	if start == nil || end == nil {
		return nil, fmt.Errorf("start and end should not be nil")
	}

	if ip.Cmp(start, end) > 0 {
		return nil, nil
	}

	return &IPRange{
		start: start,
		end:   end,
	}, nil
}

func (ir *IPRange) TryAddIP(ipAddr net.IP) (success bool) {
	if ipAddr.Equal(ip.PrevIP(ir.start)) {
		ir.start = ipAddr
		return true
	}

	if ipAddr.Equal(ip.NextIP(ir.end)) {
		ir.end = ipAddr
		return true
	}

	if ip.Cmp(ipAddr, ir.start) >= 0 && ip.Cmp(ipAddr, ir.end) <= 0 {
		// a range exist which includes this ip address
		return true
	}

	return false
}

// Translate a subnet range into a series ip block description.
func FindSubnetExcludeIPBlocks(cidr *net.IPNet, includedRanges []*IPRange, gateway net.IP,
	excludeIPs []net.IP) ([]*net.IPNet, error) {

	cidrStart := cidr.IP
	cidrEnd := LastIP(cidr)

	var excludeIPRanges []*IPRange

	sort.Slice(includedRanges, func(i, j int) bool {
		return ip.Cmp(includedRanges[i].start, includedRanges[j].start) < 0
	})

	for currentIPRangeIndex, currentIPRange := range includedRanges {
		if ip.Cmp(currentIPRange.start, cidrStart) < 0 || ip.Cmp(currentIPRange.end, cidrEnd) > 0 {
			return nil, fmt.Errorf("ip range %v~%v is out of cidr %v",
				currentIPRange.start, currentIPRange.end, cidr)
		}

		if currentIPRangeIndex < (len(includedRanges)-1) &&
			ip.Cmp(currentIPRange.end, includedRanges[currentIPRangeIndex+1].start) >= 0 {
			return nil, fmt.Errorf("ip range is overlapped for range %v~%v and %v~%v",
				currentIPRange.start, currentIPRange.end,
				includedRanges[currentIPRangeIndex+1].start, includedRanges[currentIPRangeIndex+1].end)
		}

		if currentIPRangeIndex == 0 {
			// add [cidrStart, currentRangeStartPrev] to exclude ip ranges
			currentRangeStartPrev := ip.PrevIP(currentIPRange.start)

			ipRange, err := CreateIPRange(cidrStart, currentRangeStartPrev)
			if err != nil {
				return nil, fmt.Errorf("create ip range for %v~%v failed: %v", cidrStart, currentRangeStartPrev, err)
			}

			if ipRange != nil {
				excludeIPRanges = append(excludeIPRanges, ipRange)
			}
		}

		var nextRangeStartPrev net.IP
		if currentIPRangeIndex == len(includedRanges)-1 {
			nextRangeStartPrev = cidrEnd
		} else {
			nextRangeStartPrev = ip.PrevIP(includedRanges[currentIPRangeIndex+1].start)
		}

		// add [endNext, nextRangeStartPrev] to exclude ip ranges
		endNext := ip.NextIP(currentIPRange.end)

		ipRange, err := CreateIPRange(endNext, nextRangeStartPrev)
		if err != nil {
			return nil, fmt.Errorf("create ip range for %v~%v failed: %v", endNext, nextRangeStartPrev, err)
		}

		if ipRange != nil {
			excludeIPRanges = append(excludeIPRanges, ipRange)
		}
	}

	allExcludedIPs := excludeIPs
	if gateway != nil {
		allExcludedIPs = append(allExcludedIPs, gateway)
	}

Loop2:
	for _, ipAddr := range allExcludedIPs {
		if len(excludeIPRanges) != 0 {
			for _, ipRange := range excludeIPRanges {
				if ipRange.TryAddIP(ipAddr) {
					continue Loop2
				}
			}
		}

		singleIPRange, err := CreateIPRange(ipAddr, ipAddr)
		if err != nil {
			return nil, fmt.Errorf("create ip range for ip %v failed: %v", ipAddr, err)
		}
		excludeIPRanges = append(excludeIPRanges, singleIPRange)
	}

	var excludeIPBlocks []*net.IPNet
	for _, ipRange := range excludeIPRanges {
		excludeIPBlocks = append(excludeIPBlocks, ipRange.splitIPRangeToIPBlocks()...)
	}

	return excludeIPBlocks, nil
}

func (ir *IPRange) splitIPRangeToIPBlocks() []*net.IPNet {
	rangeStart := ir.start
	rangeEnd := ir.end
	nextRangeStart := rangeStart

	var ipBlocks []*net.IPNet

	for nextRangeStart != nil {
		var newBlock *net.IPNet
		newBlock, nextRangeStart = findTheFirstLargestCidr(nextRangeStart, rangeEnd)
		ipBlocks = append(ipBlocks, newBlock)
	}

	return ipBlocks
}

func calculateIPLastZeroBits(ip net.IP) int {
	zeroBits := 0
	for {
		nextPossibleZeroBits := zeroBits + 1
		ipInt := ipToInt(ip)
		ipUint64 := ipInt.Uint64()

		if ipUint64 == (ipUint64>>nextPossibleZeroBits)<<nextPossibleZeroBits {
			zeroBits = nextPossibleZeroBits
		} else {
			break
		}
	}

	return zeroBits
}

func findTheFirstLargestCidr(start, end net.IP) (*net.IPNet, net.IP) {
	// The max possible cidr size for the start ip to represent.
	zeroBits := calculateIPLastZeroBits(start)
	var ipLen = net.IPv4len * 8
	if start.To4() == nil {
		ipLen = net.IPv6len * 8
	}

	minCidrPrefixLen := ipLen - zeroBits
	maxValidCidrPrefixLen := minCidrPrefixLen

	for {
		tmpCidr := &net.IPNet{
			IP:   start,
			Mask: net.CIDRMask(maxValidCidrPrefixLen, ipLen),
		}

		tmpCidrEnd := LastIP(tmpCidr)

		if tmpCidrEnd.Equal(end) {
			return tmpCidr, nil
		}

		if tmpCidr.Contains(end) {
			maxValidCidrPrefixLen++
		} else {
			return tmpCidr, ip.NextIP(tmpCidrEnd)
		}
	}
}

func ipToInt(ip net.IP) *big.Int {
	if v := ip.To4(); v != nil {
		return big.NewInt(0).SetBytes(v)
	}
	return big.NewInt(0).SetBytes(ip.To16())
}

func intToIP(i *big.Int) net.IP {
	return i.Bytes()
}

func LastIP(cidr *net.IPNet) net.IP {
	cidrPrefixLen, ipLen := cidr.Mask.Size()

	if cidrPrefixLen == ipLen {
		return cidr.IP
	}

	cidrEndIPInt := ipToInt(cidr.IP)
	cidrEndIPInt.Add(cidrEndIPInt, big.NewInt(1<<(ipLen-cidrPrefixLen)-1))

	return intToIP(cidrEndIPInt)
}
