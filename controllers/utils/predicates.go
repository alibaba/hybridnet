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
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
)

// IgnoreDeletePredicate will ignore the delete event, if finalizer is used,
// this predicate will help to reduce reconciliation count
type IgnoreDeletePredicate struct {
	predicate.Funcs
}

func (IgnoreDeletePredicate) Delete(e event.DeleteEvent) bool {
	return false
}

type NetworkSpecChangePredicate struct {
	predicate.Funcs
}

func (NetworkSpecChangePredicate) Update(e event.UpdateEvent) bool {
	oldNetwork, ok := e.ObjectOld.(*networkingv1.Network)
	if !ok {
		return false
	}

	newNetwork, ok := e.ObjectNew.(*networkingv1.Network)
	if !ok {
		return false
	}

	// change indicators
	// 1. netID
	// 2. node selector
	return !reflect.DeepEqual(oldNetwork.Spec.NetID, newNetwork.Spec.NetID) || !reflect.DeepEqual(oldNetwork.Spec.NodeSelector, newNetwork.Spec.NodeSelector)
}

type SubnetSpecChangePredicate struct {
	predicate.Funcs
}

func (SubnetSpecChangePredicate) Update(e event.UpdateEvent) bool {
	oldSubnet, ok := e.ObjectOld.(*networkingv1.Subnet)
	if !ok {
		return false
	}
	newSubnet, ok := e.ObjectNew.(*networkingv1.Subnet)
	if !ok {
		return false
	}

	// change indicators
	// 1. address range
	// 2. private
	return !reflect.DeepEqual(oldSubnet.Spec.Range, newSubnet.Spec.Range) || networkingv1.IsPrivateSubnet(oldSubnet) != networkingv1.IsPrivateSubnet(newSubnet)
}
