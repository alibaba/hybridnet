package controller

import (
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
)

// add handler for RemoteVtep and RemoteSubnet

func (c *Controller) enqueueAddOrDeleteRemoteVtep(obj interface{}) {
	c.nodeQueue.Add(ActionReconcileNode)
}

func (c *Controller) enqueueUpdateRemoteVtep(oldObj, newObj interface{}) {
	oldRv := oldObj.(*ramav1.RemoteVtep)
	newRv := newObj.(*ramav1.RemoteVtep)

	if oldRv.Spec.VtepIP != newRv.Spec.VtepIP ||
		oldRv.Spec.VtepMAC != newRv.Spec.VtepMAC ||
		!isIPListEqual(oldRv.Spec.EndpointIPList, newRv.Spec.EndpointIPList) {
		c.nodeQueue.Add(ActionReconcileNode)
	}
}
