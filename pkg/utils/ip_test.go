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
	"net"
	"reflect"
	"testing"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

func TestStringToIPNet(t *testing.T) {
	ipNetString := "192.168.0.100/24"
	ipNetExpected := net.IPNet{
		IP:   net.IPv4(192, 168, 0, 100),
		Mask: net.CIDRMask(24, 32),
	}

	ipNet := StringToIPNet(ipNetString)

	if !reflect.DeepEqual(*ipNet, ipNetExpected) {
		t.Errorf("test fails, expected %+v but got %+v", ipNetExpected, ipNet)
	}
}

func TestNormalizedIP(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		expected string
	}{
		{
			"ipv4",
			"192.168.0.1",
			"192.168.0.1",
		},
		{
			"ipv6",
			"2001:0410:0000:0001:0000:0000:0000:45ff",
			"2001:0410:0000:0001:0000:0000:0000:45ff",
		},
		{
			"bad ip",
			"192.168.0",
			"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if out := NormalizedIP(test.in); out != test.expected {
				t.Errorf("test %s fails: expected %s but got %s", test.name, test.expected, out)
				return
			}
		})
	}
}

func TestIntersect(t *testing.T) {
	testCase := []struct {
		name     string
		in       []v1.AddressRange
		expected bool
	}{
		{
			"two cidr",
			[]v1.AddressRange{
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
				},
				{
					Version: "4",
					CIDR:    "192.168.100.5/24",
				},
			},
			false,
		},
		{
			"same cidr, no start end",
			[]v1.AddressRange{
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
				},
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
				},
			},
			true,
		},
		{
			"same cidr, non-overlapping start end",
			[]v1.AddressRange{
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.10",
					End:     "192.168.1.50",
				},
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.51",
					End:     "192.168.1.100",
				},
			},
			false,
		},
		{
			"[1]same cidr, overlap start end",
			[]v1.AddressRange{
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.10",
					End:     "192.168.1.50",
				},
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.50",
					End:     "192.168.1.100",
				},
			},
			true,
		},
		{
			"[2]same cidr, overlap start end",
			[]v1.AddressRange{
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.50",
					End:     "192.168.1.100",
				},
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.10",
					End:     "192.168.1.50",
				},
			},
			true,
		},
		{
			"[1]same cidr, overlap start end in excludedIPs",
			[]v1.AddressRange{
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.49",
					End:        "192.168.1.100",
					ExcludeIPs: []string{"192.168.1.49", "192.168.1.50"},
				},
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.10",
					End:     "192.168.1.50",
				},
			},
			false,
		},
		{
			"[2]same cidr, overlap start end in excludedIPs",
			[]v1.AddressRange{
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.49",
					End:     "192.168.1.100",
				},
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.10",
					End:        "192.168.1.50",
					ExcludeIPs: []string{"192.168.1.49", "192.168.1.50"},
				},
			},
			false,
		},
		{
			"[3]same cidr, overlap start end in excludedIPs",
			[]v1.AddressRange{
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.50",
					End:        "192.168.1.100",
					ExcludeIPs: []string{"192.168.1.50"},
				},
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.10",
					End:        "192.168.1.49",
					ExcludeIPs: []string{"192.168.1.49"},
				},
			},
			false,
		},
		{
			"same cidr, overlap start end not in excludedIPs",
			[]v1.AddressRange{
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.49",
					End:        "192.168.1.100",
					ExcludeIPs: []string{"192.168.1.100"},
				},
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.10",
					End:        "192.168.1.50",
					ExcludeIPs: []string{"192.168.1.10"},
				},
			},
			true,
		},
		{
			"same cidr, overlap start end not in excludedIPs, the same start and end",
			[]v1.AddressRange{
				{
					Version:    "4",
					CIDR:       "192.168.1.5/24",
					Start:      "192.168.1.49",
					End:        "192.168.1.100",
					ExcludeIPs: []string{"192.168.1.100"},
				},
				{
					Version: "4",
					CIDR:    "192.168.1.5/24",
					Start:   "192.168.1.50",
					End:     "192.168.1.50",
				},
			},
			true,
		},
	}

	for _, test := range testCase {
		t.Run(test.name, func(t *testing.T) {
			if out := Intersect(&test.in[0], &test.in[1]); out != test.expected {
				t.Errorf("test %s fails: expected %v but got %v", test.name, test.expected, out)
				return
			}
		})
	}

}
