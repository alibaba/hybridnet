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

package ipam

import (
	"context"

	v1 "k8s.io/api/core/v1"

	"github.com/alibaba/hybridnet/pkg/ipam/types"
)

type Manager interface {
	Refresh(options ...types.RefreshOption) error

	GetNetworkUsage(networkName string) (*types.NetworkUsage, error)
	GetSubnetUsage(networkName, subnetName string) (*types.Usage, error)

	Allocate(networkName string, podInfo types.PodInfo, options ...types.AllocateOption) (allocatedIPs []*types.IP, err error)
	Assign(networkName string, podInfo types.PodInfo, assignedSuites []types.SubnetIPSuite, options ...types.AssignOption) (assignedIPs []*types.IP, err error)
	Release(networkName string, releaseSuites []types.SubnetIPSuite) (err error)
}

type Store interface {
	Couple(ctx context.Context, pod *v1.Pod, IPs []*types.IP, options ...types.CoupleOption) (err error)
	ReCouple(ctx context.Context, pod *v1.Pod, IPs []*types.IP, options ...types.ReCoupleOption) (err error)
	DeCouple(ctx context.Context, pod *v1.Pod) (err error)
	IPReserve(ctx context.Context, pod *v1.Pod, options ...types.ReserveOption) (err error)
	IPRecycle(ctx context.Context, namespace string, ip *types.IP) (err error)
	IPUnBind(ctx context.Context, namespace, ip string) (err error)
}
