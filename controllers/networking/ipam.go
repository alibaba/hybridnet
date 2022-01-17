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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/hybridnet/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/constants"
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

func NewIPAMManager(c client.Client) (IPAMManager, error) {
	networkList, err := utils.ListNetworks(c)
	if err != nil {
		return nil, err
	}

	var networkNames = make([]string, len(networkList.Items))
	for i := range networkList.Items {
		networkNames[i] = networkList.Items[i].Name
	}

	manager := &ipamManager{}
	if feature.DualStackEnabled() {
		manager.Interface, err = allocator.NewAllocator(networkNames, NetworkGetter(c), SubnetGetter(c), IPSetGetter(c))
		if err != nil {
			return nil, err
		}
	} else {
		manager.dualStack, err = allocator.NewDualStackAllocator(networkNames, NetworkGetter(c), SubnetGetter(c), IPSetGetter(c))
		if err != nil {
			return nil, err
		}
	}

	return manager, nil
}

func NetworkGetter(c client.Client) allocator.NetworkGetter {
	return func(networkName string) (*types.Network, error) {
		network, err := utils.GetNetwork(c, networkName)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}

		return transform.TransferNetworkForIPAM(network), nil
	}
}

func SubnetGetter(c client.Client) allocator.SubnetGetter {
	return func(networkName string) ([]*types.Subnet, error) {
		subnetList, err := utils.ListSubnets(c)
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

func IPSetGetter(c client.Client) allocator.IPSetGetter {
	return func(subnetName string) (ipamtypes.IPSet, error) {
		ipList, err := utils.ListIPInstances(c, client.MatchingLabels{
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
