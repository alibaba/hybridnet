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
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
)

// add handler for RemoteVtep and RemoteSubnet

func (c *Controller) enqueueAddOrDeleteRemoteVtep(obj interface{}) {
	c.nodeQueue.Add(ActionReconcileNode)
}

func (c *Controller) enqueueUpdateRemoteVtep(oldObj, newObj interface{}) {
	old := oldObj.(*networkingv1.RemoteVtep)
	new := newObj.(*networkingv1.RemoteVtep)

	if old.Spec.VtepIP != new.Spec.VtepIP ||
		old.Spec.VtepMAC != new.Spec.VtepMAC ||
		old.Annotations[constants.AnnotationNodeLocalVxlanIPList] != new.Annotations[constants.AnnotationNodeLocalVxlanIPList] ||
		!isIPListEqual(old.Spec.EndpointIPList, new.Spec.EndpointIPList) {
		c.nodeQueue.Add(ActionReconcileNode)
	}
}
