package controller

import (
	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/utils"
)

func (c *Controller) enqueueAddOrDeleteRemoteSubnet(obj interface{}) {
	c.subnetQueue.Add(ActionReconcileSubnet)
}

func (c *Controller) enqueueUpdateRemoteSubnet(oldObj, newObj interface{}) {
	oldRs := oldObj.(*networkingv1.RemoteSubnet)
	newRs := newObj.(*networkingv1.RemoteSubnet)

	if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
		!utils.Intersect(&oldRs.Spec.Range, &newRs.Spec.Range) ||
		networkingv1.GetRemoteSubnetType(oldRs) != networkingv1.GetRemoteSubnetType(newRs) {
		c.subnetQueue.Add(ActionReconcileSubnet)
	}
}
