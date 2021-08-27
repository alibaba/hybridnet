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
