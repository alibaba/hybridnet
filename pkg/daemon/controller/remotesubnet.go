package controller

import (
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
)

func (c *Controller) enqueueAddOrDeleteRemoteSubnet(obj interface{}) {
	c.subnetQueue.Add(ActionReconcileSubnet)
}

func (c *Controller) enqueueUpdateRemoteSubnet(oldObj, newObj interface{}) {
	oldRs := oldObj.(*ramav1.RemoteSubnet)
	newRs := newObj.(*ramav1.RemoteSubnet)

	if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
		!isAddressRangeEqual(&oldRs.Spec.Range, &newRs.Spec.Range) ||
		ramav1.GetRemoteSubnetType(oldRs) != ramav1.GetRemoteSubnetType(newRs) {
		c.subnetQueue.Add(ActionReconcileSubnet)
	}
}
