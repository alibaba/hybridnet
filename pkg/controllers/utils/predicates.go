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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	multiclusterv1 "github.com/alibaba/hybridnet/apis/multicluster/v1"
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

// IgnoreUpdatePredicate will ignore the update event
type IgnoreUpdatePredicate struct {
	predicate.Funcs
}

func (IgnoreUpdatePredicate) Update(e event.UpdateEvent) bool {
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

type NetworkStatusChangePredicate struct {
	predicate.Funcs
}

func (NetworkStatusChangePredicate) Update(e event.UpdateEvent) bool {
	oldNetwork, ok := e.ObjectOld.(*networkingv1.Network)
	if !ok {
		return false
	}

	newNetwork, ok := e.ObjectNew.(*networkingv1.Network)
	if !ok {
		return false
	}

	// change indicators
	// 1. node list
	// 2. statistics change between zero and non-zero
	if !reflect.DeepEqual(oldNetwork.Status.NodeList, newNetwork.Status.NodeList) {
		return true
	}
	if networkingv1.IsAvailable(oldNetwork.Status.Statistics) != networkingv1.IsAvailable(newNetwork.Status.Statistics) {
		return true
	}
	if networkingv1.IsAvailable(oldNetwork.Status.IPv6Statistics) != networkingv1.IsAvailable(newNetwork.Status.IPv6Statistics) {
		return true
	}
	if networkingv1.IsAvailable(oldNetwork.Status.DualStackStatistics) != networkingv1.IsAvailable(newNetwork.Status.DualStackStatistics) {
		return true
	}
	return false
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

type NetworkOfNodeChangePredicate struct {
	Client client.Client
	predicate.Funcs
}

func (n NetworkOfNodeChangePredicate) Update(e event.UpdateEvent) bool {
	oldNetwork, err := FindUnderlayNetworkForNode(n.Client, e.ObjectOld.GetLabels())
	if err != nil {
		// TODO: log here
		return true
	}
	newNetwork, err := FindUnderlayNetworkForNode(n.Client, e.ObjectNew.GetLabels())
	if err != nil {
		// TODO: log here
		return true
	}

	return newNetwork != oldNetwork
}

type SpecifiedAnnotationChangedPredicate struct {
	predicate.Funcs
	AnnotationKeys []string
}

// Update implements default UpdateEvent filter for validating specified annotations change
func (s SpecifiedAnnotationChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		return false
	}
	if e.ObjectNew == nil {
		return false
	}

	for _, annotationKey := range s.AnnotationKeys {
		if e.ObjectNew.GetAnnotations()[annotationKey] != e.ObjectOld.GetAnnotations()[annotationKey] {
			return true
		}
	}
	return false
}

type SpecifiedLabelChangedPredicate struct {
	predicate.Funcs
	LabelKeys []string
}

// Update implements default UpdateEvent filter for validating specified annotations change
func (s SpecifiedLabelChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		return false
	}
	if e.ObjectNew == nil {
		return false
	}

	for _, labelKey := range s.LabelKeys {
		if e.ObjectNew.GetLabels()[labelKey] != e.ObjectOld.GetLabels()[labelKey] {
			return true
		}
	}
	return false
}

type IPInstancePhaseChangePredicate struct {
	predicate.Funcs
}

// Update implements default UpdateEvent filter for checking whether IPInstance phase change
func (IPInstancePhaseChangePredicate) Update(e event.UpdateEvent) bool {
	oldIPInstance, ok := e.ObjectOld.(*networkingv1.IPInstance)
	if !ok {
		return false
	}
	newIPInstance, ok := e.ObjectNew.(*networkingv1.IPInstance)
	if !ok {
		return false
	}

	return oldIPInstance.Status.Phase != newIPInstance.Status.Phase
}

type RemoteClusterUUIDChangePredicate struct {
	predicate.Funcs
}

func (RemoteClusterUUIDChangePredicate) Update(e event.UpdateEvent) bool {
	oldRemoteCluster, ok := e.ObjectOld.(*multiclusterv1.RemoteCluster)
	if !ok {
		return false
	}
	newRemoteCluster, ok := e.ObjectNew.(*multiclusterv1.RemoteCluster)
	if !ok {
		return false
	}

	return oldRemoteCluster.Status.UUID != newRemoteCluster.Status.UUID
}
