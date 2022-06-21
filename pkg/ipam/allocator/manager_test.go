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

package allocator_test

import (
	"fmt"
	"net"
	"testing"

	apitypes "k8s.io/apimachinery/pkg/types"

	"github.com/alibaba/hybridnet/pkg/ipam/allocator"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
)

func TestManager(t *testing.T) {
	var networkGetter = func(network string) (*types.Network, error) {
		return &types.Network{
			Name:                network,
			NetID:               nil,
			LastAllocatedSubnet: "subnet1",
			Subnets:             types.NewSubnetSlice(),
			Type:                types.Underlay,
		}, nil
	}

	var subnetGetter = func(networkName string) ([]*types.Subnet, error) {
		subnetIndex := 0
		newSubnet := func(gateway, cidr string, netID uint32) *types.Subnet {
			_, cidrNet, _ := net.ParseCIDR(cidr)
			subnetIndex++
			return types.NewSubnet(
				fmt.Sprintf("subnet%d", subnetIndex),
				networkName,
				generatePointerInt(netID),
				nil,
				nil,
				net.ParseIP(gateway),
				cidrNet,
				nil,
				nil,
				nil,
				false,
				cidrNet.IP.To4() == nil,
			)
		}

		return append([]*types.Subnet{},
			newSubnet("172.168.0.254", "172.168.0.0/24", 60),
			newSubnet("192.168.0.254", "192.168.0.0/24", 100),
			newSubnet("2048::0001", "2048::0000/120", 100),
			newSubnet("2047::0001", "2047::0000/120", 70),
		), nil
	}

	var ipSetGetter = func(subnet string) (types.IPSet, error) {
		return types.NewIPSet(), nil
	}

	networkTest := "network-test-1"
	manager, err := allocator.NewManager([]string{networkTest}, networkGetter, subnetGetter, ipSetGetter)
	if err != nil {
		t.Errorf("fail to new manager: %v", err)
		return
	}

	count := 100

	var ipFamilyMode types.IPFamilyMode
	for {
		switch count % 3 {
		case 0:
			ipFamilyMode = types.IPv4Only
		case 1:
			ipFamilyMode = types.IPv6Only
		case 2:
			ipFamilyMode = types.DualStack
		}

		ips, err := manager.Allocate(networkTest, types.PodInfo{
			NamespacedName: apitypes.NamespacedName{
				Namespace: "testns",
				Name:      "testname",
			},
			IPFamily: ipFamilyMode,
		})
		if err != nil {
			t.Errorf("fail to allocate ip %s: %v", ipFamilyMode, err)
			return
		}

		t.Logf("allocate ips %+v when %s mode", ips, ipFamilyMode)

		count--
		if count == 0 {
			break
		}
	}
}

func generatePointerInt(a uint32) *uint32 {
	return &a
}
