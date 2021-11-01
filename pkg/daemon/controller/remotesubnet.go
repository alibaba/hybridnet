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

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

func (c *Controller) enqueueAddOrDeleteRemoteSubnet(obj interface{}) {
	c.subnetQueue.Add(ActionReconcileSubnet)
}

func (c *Controller) enqueueUpdateRemoteSubnet(oldObj, newObj interface{}) {
	oldRs := oldObj.(*networkingv1.RemoteSubnet)
	newRs := newObj.(*networkingv1.RemoteSubnet)

	if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
		!reflect.DeepEqual(oldRs.Spec.Range, newRs.Spec.Range) ||
		networkingv1.GetRemoteSubnetType(oldRs) != networkingv1.GetRemoteSubnetType(newRs) {
		c.subnetQueue.Add(ActionReconcileSubnet)
	}
}
