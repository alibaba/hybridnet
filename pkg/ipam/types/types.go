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

import "net"

const (
	IPStatusAllocated = "Allocated"
	IPStatusReserved  = "Reserved"
)

type Network struct {
	// Spec fields
	Name                string
	NetID               *uint32
	LastAllocatedSubnet string
	Type                NetworkType

	Subnets *SubnetSlice
}

type NetworkSet map[string]*Network

type Subnet struct {
	// Spec fields
	// `Canonicalize` method will initialize these
	Name            string
	ParentNetwork   string
	NetID           *uint32
	Start           net.IP
	End             net.IP
	CIDR            *net.IPNet
	Gateway         net.IP
	ReservedList    map[string]struct{}
	BlackList       map[string]struct{}
	LastAllocatedIP net.IP
	Private         bool
	IPv6            bool

	// Status fields
	// `Sync` method will initialize these
	AvailableIPs    *IPSlice
	UsingIPs        IPSet
	ReservedIPCount int
}

type SubnetSlice struct {
	Subnets        []*Subnet
	SubnetIndexMap map[string]int

	SubnetIndex int
	SubnetCount int
}

type IP struct {
	Address *net.IPNet
	Gateway net.IP
	NetID   *uint32
	Subnet  string
	Network string

	PodName      string
	PodNamespace string

	Status string
}

type IPSet map[string]*IP

type IPSlice struct {
	IPs []string

	IPCount int
	IPIndex int
}
