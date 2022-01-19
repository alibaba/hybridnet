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
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/handler"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
)

type enqueueRequestForRemoteSubnet struct {
	handler.Funcs
}

func (e enqueueRequestForRemoteSubnet) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcileSubnetRequest)
}

func (e enqueueRequestForRemoteSubnet) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if evt.ObjectOld == nil || evt.ObjectNew == nil {
		return
	}

	oldRs := evt.ObjectOld.(*multiclusterv1.RemoteSubnet)
	newRs := evt.ObjectNew.(*multiclusterv1.RemoteSubnet)

	if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
		!reflect.DeepEqual(oldRs.Spec.Range, newRs.Spec.Range) ||
		multiclusterv1.GetRemoteSubnetType(oldRs) != multiclusterv1.GetRemoteSubnetType(newRs) {
		q.Add(reconcileSubnetRequest)
	}
}

func (e enqueueRequestForRemoteSubnet) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	q.Add(reconcileSubnetRequest)
}
