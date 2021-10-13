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
	"reflect"

	"k8s.io/apimachinery/pkg/labels"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/transform"
)

func (c *Controller) filterSubnet(obj interface{}) bool {
	_, ok := obj.(*v1.Subnet)
	return ok
}

func (c *Controller) addSubnet(obj interface{}) {
	s, ok := obj.(*v1.Subnet)
	if !ok {
		return
	}

	c.enqueueNetwork(s.Spec.Network)
}

func (c *Controller) updateSubnet(oldObj, newObj interface{}) {
	old, ok := oldObj.(*v1.Subnet)
	if !ok {
		return
	}
	new, ok := newObj.(*v1.Subnet)
	if !ok {
		return
	}

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	if old.Generation == new.Generation {
		return
	}

	if !subnetSpecChanged(&old.Spec, &new.Spec) {
		return
	}

	c.enqueueNetwork(new.Spec.Network)
}

func (c *Controller) delSubnet(obj interface{}) {
	n, ok := obj.(*v1.Subnet)
	if !ok {
		return
	}

	c.enqueueNetwork(n.Spec.Network)
}

// This function only care about the changed fields
// which are related to IPAM
func subnetSpecChanged(old, new *v1.SubnetSpec) bool {
	return !reflect.DeepEqual(old.Range, new.Range) || v1.IsPrivateSubnet(old) != v1.IsPrivateSubnet(new)
}

func (c *Controller) subnetGetter(network string) ([]*ipamtypes.Subnet, error) {
	subnetList, err := c.subnetLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var subnets []*ipamtypes.Subnet
	for _, item := range subnetList {
		if item.Spec.Network == network {
			subnets = append(subnets, transform.TransferSubnetForIPAM(item))
		}
	}
	return subnets, nil
}
