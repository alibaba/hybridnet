package controller

import (
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
)

// add handler for RemoteVtep and RemoteSubnet

func (c *Controller) enqueueAddOrDeleteRemoteVtep(obj interface{}) {
	c.nodeQueue.Add(ActionReconcileNode)
}

func (c *Controller) enqueueUpdateRemoteVtep(oldObj, newObj interface{}) {
	old := oldObj.(*ramav1.RemoteVtep)
	new := newObj.(*ramav1.RemoteVtep)

	if old.Spec.VtepIP != new.Spec.VtepIP ||
		old.Spec.VtepMAC != new.Spec.VtepMAC ||
		old.Annotations[constants.AnnotationNodeLocalVxlanIPList] != new.Annotations[constants.AnnotationNodeLocalVxlanIPList] ||
		!isIPListEqual(old.Spec.EndpointIPList, new.Spec.EndpointIPList) {
		c.nodeQueue.Add(ActionReconcileNode)
	}
}
