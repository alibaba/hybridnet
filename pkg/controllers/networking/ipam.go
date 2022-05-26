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

package networking

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam"
	"github.com/alibaba/hybridnet/pkg/ipam/allocator"
	"github.com/alibaba/hybridnet/pkg/ipam/store"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/transform"
)

type IPAMManager interface {
	ipam.Interface
	DualStack() ipam.DualStackInterface
}

func NewIPAMManager(ctx context.Context, c client.Reader) (IPAMManager, error) {
	networkList, err := utils.ListNetworks(ctx, c)
	if err != nil {
		return nil, err
	}

	var networkNames = make([]string, len(networkList.Items))
	for i := range networkList.Items {
		networkNames[i] = networkList.Items[i].Name
	}

	manager := &ipamManager{}
	if feature.DualStackEnabled() {
		manager.dualStack, err = allocator.NewDualStackAllocator(networkNames,
			NetworkGetter(ctx, c), SubnetGetter(ctx, c), IPSetGetter(ctx, c))
		if err != nil {
			return nil, err
		}
	} else {
		manager.Interface, err = allocator.NewAllocator(networkNames,
			NetworkGetter(ctx, c), SubnetGetter(ctx, c), IPSetGetter(ctx, c))
		if err != nil {
			return nil, err
		}
	}

	return manager, nil
}

func NetworkGetter(ctx context.Context, c client.Reader) allocator.NetworkGetter {
	return func(networkName string) (*types.Network, error) {
		network, err := utils.GetNetwork(ctx, c, networkName)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}

		return transform.TransferNetworkForIPAM(network), nil
	}
}

func SubnetGetter(ctx context.Context, c client.Reader) allocator.SubnetGetter {
	return func(networkName string) ([]*types.Subnet, error) {
		subnetList, err := utils.ListSubnets(ctx, c)
		if err != nil {
			return nil, err
		}

		var subnets []*ipamtypes.Subnet
		for i := range subnetList.Items {
			subnet := &subnetList.Items[i]
			if subnet.Spec.Network == networkName {
				subnets = append(subnets, transform.TransferSubnetForIPAM(subnet))
			}
		}
		return subnets, nil
	}
}

func IPSetGetter(ctx context.Context, c client.Reader) allocator.IPSetGetter {
	return func(subnetName string) (ipamtypes.IPSet, error) {
		ipList, err := utils.ListIPInstances(ctx, c, client.MatchingLabels{
			constants.LabelSubnet: subnetName,
		})
		if err != nil {
			return nil, err
		}

		// TODO: init with capacity
		ipSet := ipamtypes.NewIPSet()
		for i := range ipList.Items {
			ip := &ipList.Items[i]
			ipSet.Add(utils.ToIPFormat(ip.Name), transform.TransferIPInstanceForIPAM(ip))
		}
		return ipSet, nil
	}
}

type ipamManager struct {
	ipam.Interface
	dualStack ipam.DualStackInterface
}

func (i *ipamManager) DualStack() ipam.DualStackInterface {
	return i.dualStack
}

func (i *ipamManager) Refresh(networks []string) error {
	if feature.DualStackEnabled() {
		return i.DualStack().Refresh(networks)
	}
	return i.Interface.Refresh(networks)
}

type IPAMStore interface {
	ipam.Store
	DualStack() ipam.DualStackStore
}

type ipamStore struct {
	ipam.Store
	dualStack ipam.DualStackStore
}

func (i *ipamStore) DualStack() ipam.DualStackStore {
	return i.dualStack
}

func NewIPAMStore(c client.Client) IPAMStore {
	return &ipamStore{
		Store:     store.NewWorker(c),
		dualStack: store.NewDualStackWorker(c),
	}
}
