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

package types

import (
	"net"
	"testing"
)

func TestIPSet(t *testing.T) {
	ipset := NewIPSet()

	subnet := "subnet"
	network := "network"

	mask := net.IPv4Mask(byte(255), byte(255), byte(255), byte(0))

	ip1 := net.ParseIP("192.168.1.125")
	ip2 := net.ParseIP("192.168.1.126")
	ip3 := net.ParseIP("192.168.1.127")

	gateway := net.ParseIP("192.168.1.254")

	var netID uint32 = 10

	testIPs := []*IP{
		{
			Address: &net.IPNet{
				IP:   ip1,
				Mask: mask,
			},
			Gateway:      gateway,
			NetID:        &netID,
			Subnet:       subnet,
			Network:      network,
			PodName:      "pod1",
			PodNamespace: "namespace1",
			Status:       "Using",
		},
		{
			Address: &net.IPNet{
				IP:   ip2,
				Mask: mask,
			},
			Gateway:      gateway,
			NetID:        &netID,
			Subnet:       subnet,
			Network:      network,
			PodName:      "pod2",
			PodNamespace: "namespace2",
			Status:       "Using",
		},
		{
			Address: &net.IPNet{
				IP:   ip3,
				Mask: mask,
			},
			Gateway:      gateway,
			NetID:        &netID,
			Subnet:       subnet,
			Network:      network,
			PodName:      "pod3",
			PodNamespace: "namespace3",
			Status:       "Using",
		},
	}

	for _, ip := range testIPs {
		ipset.Add(ip.Address.IP.String(), ip)
	}

	for _, ip := range testIPs {
		if !ipset.Has(ip.Address.IP.String()) {
			t.Fatalf("ip %v not exist in ipset", ip.Address.IP.String())
		}
	}

	ipset.Update(testIPs[0].Address.IP.String(), "p1", "ns1", "Failed")

	ip := ipset.Get(testIPs[0].Address.IP.String())
	if ip.PodName != "p1" || ip.PodNamespace != "ns1" {
		t.Fatalf("failed to update ip %v", testIPs[0].Address.IP.String())
	}

	ipset.Delete(testIPs[0].Address.IP.String())
	if ipset.Count() != 2 {
		t.Fatalf("failed to delete ip %v", testIPs[0].Address.IP.String())
	}
}

func TestIPSlice(t *testing.T) {
	ipSlice := NewIPSlice()

	ips := []string{
		"192.168.1.125",
		"192.168.1.126",
		"192.168.1.127",
	}

	for _, ip := range ips {
		ipSlice.Add(ip, false)
	}

	ipSlice.Add("192.168.1.128", true)

	if ipSlice.Count() != 4 {
		t.Fatal("failed to add ip to slice")
	}

	for i := 0; i < 2; i++ {
		ipSlice.Next()
	}

	if ipSlice.Current() != ips[1] {
		t.Fatal("failed to get next ip for slice")
	}
}

func TestIP(t *testing.T) {
	var netID uint32 = 10

	ip := IP{
		Address: &net.IPNet{
			IP:   net.ParseIP("192.168.1.125"),
			Mask: net.IPv4Mask(byte(255), byte(255), byte(255), byte(0)),
		},
		Gateway:      net.ParseIP("192.168.1.254"),
		NetID:        &netID,
		Subnet:       "subnet",
		Network:      "network",
		PodName:      "pod1",
		PodNamespace: "namespace1",
		Status:       "Using",
	}

	if ip.String() != "network/subnet/192.168.1.125/24" {
		t.Fatal("ip format error")
	}
}
