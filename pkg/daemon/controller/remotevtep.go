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
