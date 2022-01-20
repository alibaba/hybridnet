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

package controller

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

type enqueueRequestForNetwork struct {
	handler.Funcs
}

func (e enqueueRequestForNetwork) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcileSubnetRequest)
}

func (e enqueueRequestForNetwork) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if evt.ObjectOld == nil || evt.ObjectNew == nil {
		return
	}

	oldNetwork := evt.ObjectOld.(*networkingv1.Network)
	newNetwork := evt.ObjectNew.(*networkingv1.Network)

	if len(oldNetwork.Status.SubnetList) != len(newNetwork.Status.SubnetList) ||
		len(oldNetwork.Status.NodeList) != len(newNetwork.Status.NodeList) {
		q.Add(reconcileSubnetRequest)
		return
	}

	for index, subnet := range oldNetwork.Status.SubnetList {
		if subnet != newNetwork.Status.SubnetList[index] {
			q.Add(reconcileSubnetRequest)
			return
		}
	}

	for index, node := range oldNetwork.Status.NodeList {
		if node != newNetwork.Status.NodeList[index] {
			q.Add(reconcileSubnetRequest)
			return
		}
	}
}

func (e enqueueRequestForNetwork) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcileSubnetRequest)
}

// reconcile subnet and node info on node if network info changed
func enqueueAddOrDeleteNetworkForNode(obj client.Object, q workqueue.RateLimitingInterface) {
	if obj != nil {
		network := obj.(*networkingv1.Network)
		if networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay {
			q.Add(reconcileNodeRequest)
		}
	}
}
