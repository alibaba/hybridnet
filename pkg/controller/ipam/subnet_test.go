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

package ipam

import (
	"testing"

	v1 "github.com/oecp/rama/pkg/apis/networking/v1"
)

func TestSubnetSpecChanged(t *testing.T) {
	tests := []struct {
		name    string
		old     v1.SubnetSpec
		new     v1.SubnetSpec
		changed bool
	}{

		{
			"empty",
			v1.SubnetSpec{},
			v1.SubnetSpec{},
			false,
		},
		{
			"version changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv6,
				},
			},
			true,
		},
		{
			"start changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.2",
				},
			},
			true,
		},
		{
			"end changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
					End:     "192.168.0.100",
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
					End:     "192.168.0.101",
				},
			},
			true,
		},
		{
			"cidr changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
					End:     "192.168.0.100",
					CIDR:    "192.168.0.0/24",
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
					End:     "192.168.0.100",
					CIDR:    "192.168.0.0/23",
				},
			},
			true,
		},
		{
			"gateway changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
					End:     "192.168.0.100",
					CIDR:    "192.168.0.0/24",
					Gateway: "192.168.0.254",
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version: v1.IPv4,
					Start:   "192.168.0.1",
					End:     "192.168.0.100",
					CIDR:    "192.168.0.0/24",
					Gateway: "192.168.0.253",
				},
			},
			true,
		},
		{
			"exclude ips changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version:    v1.IPv4,
					Start:      "192.168.0.1",
					End:        "192.168.0.100",
					CIDR:       "192.168.0.0/24",
					Gateway:    "192.168.0.254",
					ExcludeIPs: nil,
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version:    v1.IPv4,
					Start:      "192.168.0.1",
					End:        "192.168.0.100",
					CIDR:       "192.168.0.0/24",
					Gateway:    "192.168.0.253",
					ExcludeIPs: []string{"a", "b"},
				},
			},
			true,
		},
		{
			"private config not change with nil",
			v1.SubnetSpec{
				Config: &v1.SubnetConfig{
					Private: nil,
				},
			},
			v1.SubnetSpec{},
			false,
		},
		{
			"private config not change with value",
			v1.SubnetSpec{
				Config: &v1.SubnetConfig{
					Private: nil,
				},
			},
			v1.SubnetSpec{
				Config: &v1.SubnetConfig{
					Private: boolPointer(false),
				},
			},
			false,
		},
		{
			"private config change",
			v1.SubnetSpec{
				Config: &v1.SubnetConfig{
					Private: nil,
				},
			},
			v1.SubnetSpec{
				Config: &v1.SubnetConfig{
					Private: boolPointer(true),
				},
			},
			true,
		},
		{
			"nothing changed",
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version:    v1.IPv4,
					Start:      "192.168.0.1",
					End:        "192.168.0.100",
					CIDR:       "192.168.0.0/24",
					Gateway:    "192.168.0.254",
					ExcludeIPs: []string{"b", "a"},
				},
			},
			v1.SubnetSpec{
				Range: v1.AddressRange{
					Version:    v1.IPv4,
					Start:      "192.168.0.1",
					End:        "192.168.0.100",
					CIDR:       "192.168.0.0/24",
					Gateway:    "192.168.0.254",
					ExcludeIPs: []string{"b", "a"},
				},
			},
			false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if subnetSpecChanged(&test.old, &test.new) != test.changed {
				t.Errorf("test %s fails", test.name)
				return
			}
		})
	}
}

func boolPointer(in bool) *bool {
	return &in
}
