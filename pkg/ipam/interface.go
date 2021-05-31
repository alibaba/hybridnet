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
	v1 "k8s.io/api/core/v1"

	"github.com/oecp/rama/pkg/ipam/types"
)

type Interface interface {
	Refresh
	Usage
	NetworkInterface

	Allocate(network, subnet, podName, podNamespace string) (*types.IP, error)
	Assign(network, subnet, podname, podNamespace, ip string, forced bool) (*types.IP, error)
	Release(network, subnet, ip string) error
}

type Refresh interface {
	Refresh(networks []string) error
}

type Usage interface {
	Usage(network string) (*types.Usage, map[string]*types.Usage, error)
}

type DualStackInterface interface {
	Refresh
	DualStackUsage
	NetworkInterface

	Allocate(ipFamilyMode types.IPFamilyMode, network string, subnets []string,
		podName, podNamespace string) (IPs []*types.IP, err error)
	Assign(ipFamilyMode types.IPFamilyMode, network string, subnets, IPs []string,
		podName, podNamespace string, forced bool) (AssignedIPs []*types.IP, err error)
	Release(ipFamilyMode types.IPFamilyMode, network string, subnets, IPs []string) (err error)
}

type DualStackUsage interface {
	Usage(network string) ([3]*types.Usage, map[string]*types.Usage, error)
}

type Store interface {
	Couple(pod *v1.Pod, ip *types.IP) (err error)
	ReCouple(pod *v1.Pod, ip *types.IP) (err error)
	IPUnBind(namespace, ip string) (err error)
	SyncNetworkUsage(name string, usage *types.Usage) (err error)
	SyncSubnetUsage(name string, usage *types.Usage) (err error)
	SyncNetworkStatus(name, nodes, subnets string) (err error)
}

type DualStackStore interface {
	Couple(pod *v1.Pod, IPs []*types.IP) (err error)
	ReCouple(pod *v1.Pod, IPs []*types.IP) (err error)
	IPUnBind(namespace, ip string) (err error)
	SyncNetworkUsage(name string, usages [3]*types.Usage) (err error)
	SyncSubnetUsage(name string, usage *types.Usage) (err error)
	SyncNetworkStatus(name, nodes, subnets string) (err error)
}

type NetworkInterface interface {
	GetNetworksByType(networkType types.NetworkType) []string
	MatchNetworkType(networkName string, networkType types.NetworkType) bool
}
