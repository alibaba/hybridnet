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
	"math"
	"testing"
)

func TestCalculateCapacity(t *testing.T) {
	tests := []struct {
		name         string
		addressRange *AddressRange
		expected     int64
	}{
		{
			"invalid cidr",
			&AddressRange{
				CIDR: "fake",
			},
			math.MaxInt64,
		},
		{
			"only cidr",
			&AddressRange{
				CIDR: "192.168.0.0/24",
			},
			254,
		},
		{
			"cidr and start",
			&AddressRange{
				Start: "192.168.0.100",
				CIDR:  "192.168.0.0/24",
			},
			155,
		},
		{
			"cidr, start and end",
			&AddressRange{
				Start: "192.168.0.100",
				End:   "192.168.0.200",
				CIDR:  "192.168.0.0/24",
			},
			101,
		},
		{
			"all set",
			&AddressRange{
				Start: "192.168.0.100",
				End:   "192.168.0.200",
				CIDR:  "192.168.0.0/24",
				ExcludeIPs: []string{
					"192.168.0.105",
					"192.168.0.107",
				},
			},
			99,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if calculated := CalculateCapacity(test.addressRange); calculated != test.expected {
				t.Fatalf("test %s fails, expected %d but calculated %d", test.name, test.expected, calculated)
			}
		})
	}
}

func TestGetNetworkType(t *testing.T) {
	tests := []struct {
		name        string
		network     *Network
		networkType NetworkType
	}{
		{
			"empty",
			&Network{
				Spec: NetworkSpec{
					Type: "",
				},
			},
			NetworkTypeUnderlay,
		},
		{
			"nil",
			nil,
			NetworkTypeUnderlay,
		},
		{
			"overlay",
			&Network{
				Spec: NetworkSpec{
					Type: "Overlay",
				},
			},
			NetworkTypeOverlay,
		},
		{
			"others",
			&Network{
				Spec: NetworkSpec{
					Type: "others",
				},
			},
			NetworkType("others"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if GetNetworkType(test.network) != test.networkType {
				t.Errorf("test %s fails, expect %s but got %s", test.name, test.networkType, GetNetworkType(test.network))
			}
		})
	}
}
