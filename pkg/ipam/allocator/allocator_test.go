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
	"net"
	"testing"

	"github.com/alibaba/hybridnet/pkg/ipam/allocator"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
)

func generatePointerInt(a uint32) *uint32 {
	return &a
}

func TestAllocator(t *testing.T) {
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
		_, cidr, _ := net.ParseCIDR("192.168.0.0/24")
		gateway := net.ParseIP("192.168.0.254")
		lastAllocatedIP := net.ParseIP("192.168.0.3")

		return []*types.Subnet{
			{
				Name:          "subnet1",
				ParentNetwork: networkName,
				NetID:         generatePointerInt(100),
				Start:         nil,
				End:           nil,
				CIDR:          cidr,
				Gateway:       gateway,
				BlackList: map[string]struct{}{
					"192.168.0.5":    {},
					"192.168.0.10":   {},
					"172.16.100.200": {},
				},
				LastAllocatedIP: lastAllocatedIP,
			},
		}, nil
	}

	var ipSetGetter = func(subnet string) (types.IPSet, error) {
		return types.NewIPSet(), nil
	}

	networkTest := "network-test-1"
	allocator, err := allocator.NewAllocator([]string{networkTest}, networkGetter, subnetGetter, ipSetGetter)
	if err != nil {
		t.Errorf("fail to new allocator: %v", err)
		return
	}

	count := 10

	for {
		ip, err := allocator.Allocate(networkTest, "", "hah", "hehe")
		if err != nil {
			t.Errorf("fail to allocate ip: %v", err)
			return
		}

		t.Logf("allocate ip %+v", ip)

		count--
		if count == 0 {
			break
		}
	}

}
