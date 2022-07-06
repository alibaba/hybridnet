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

import (
	"net"
	"testing"
)

func TestSubnetSlice_CurrentSubnetName(t *testing.T) {
	tests := []struct {
		name                  string
		ss                    *SubnetSlice
		expectedCurrentSubnet string
	}{
		{
			"normal case",
			&SubnetSlice{
				Subnets: []*Subnet{
					{
						Name: "abc",
					},
					{
						Name: "def",
					},
				},
				SubnetIndex: 1,
				SubnetCount: 2,
			},
			"def",
		},
		{
			"empty subnet slice",
			&SubnetSlice{
				SubnetCount: 0,
			},
			"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.ss.CurrentSubnetName() != test.expectedCurrentSubnet {
				t.Errorf("test %s fails: expected %s but got %s", test.name, test.expectedCurrentSubnet, test.ss.CurrentSubnetName())
				return
			}
		})
	}
}

func TestSubnet_AllocateNext(t *testing.T) {
	var err error
	var cidr *net.IPNet
	var ip net.IP

	ip, cidr, _ = net.ParseCIDR("234e:0:4567::3d/120")
	subnet := NewSubnet("test", "fake", nil, nil, nil, ip, cidr, nil, nil, nil, false, false)
	if err = subnet.Canonicalize(); err != nil {
		t.Fatalf("fail to canonicalize: %v", err)
	}
	if err = subnet.Sync(nil, NewIPSet()); err != nil {
		t.Fatalf("fail to sync: %v", err)
	}

	for i := 0; i < 100; i++ {
		allocatedIP := subnet.AllocateNext("", "")
		if allocatedIP == nil {
			t.Fatalf("fail to allocate the %d ip", i)
		}
		t.Logf("the %d ip is %s", i, allocatedIP)
	}
}
