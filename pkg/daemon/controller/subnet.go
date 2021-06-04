/*
Copyright 2021 The Rama Authors.

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
	"fmt"
	"net"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/daemon/containernetwork"

	"github.com/vishvananda/netlink"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

func (c *Controller) enqueueAddOrDeleteSubnet(obj interface{}) {
	c.subnetQueue.Add(ActionReconcileSubnet)
}

func (c *Controller) enqueueUpdateSubnet(oldObj, newObj interface{}) {
	oldSubnet := oldObj.(*ramav1.Subnet)
	newSubnet := newObj.(*ramav1.Subnet)

	oldSubnetNetID := oldSubnet.Spec.NetID
	newSubnetNetID := newSubnet.Spec.NetID

	if (oldSubnetNetID == nil && newSubnetNetID != nil) ||
		(oldSubnetNetID != nil && newSubnetNetID == nil) ||
		(oldSubnetNetID != nil && newSubnetNetID != nil && *oldSubnetNetID != *newSubnetNetID) ||
		oldSubnet.Spec.Network != newSubnet.Spec.Network ||
		ramav1.IsSubnetAutoNatOutgoing(&oldSubnet.Spec) != ramav1.IsSubnetAutoNatOutgoing(&newSubnet.Spec) {
		c.subnetQueue.Add(ActionReconcileSubnet)
	}
}

func (c *Controller) processNextSubnetWorkItem() bool {
	obj, shutdown := c.subnetQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.subnetQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.subnetQueue.Forget(obj)
			klog.Errorf("expected string in work queue but got %#v", obj)
			return nil
		}
		if err := c.reconcileSubnet(); err != nil {
			c.subnetQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.subnetQueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}
	return true
}

func (c *Controller) runSubnetWorker() {
	for c.processNextSubnetWorkItem() {
	}
}

func (c *Controller) reconcileSubnet() error {
	klog.Info("Reconciling subnet information")

	subnetList, err := c.subnetLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list subnet %v", err)
	}

	c.routeV4Manager.ResetInfos()
	c.routeV6Manager.ResetInfos()

	for _, subnet := range subnetList {
		network, err := c.networkLister.Get(subnet.Spec.Network)
		if err != nil {
			return fmt.Errorf("failed to get network for subnet %v", subnet.Name)
		}

		if ramav1.GetNetworkType(network) == ramav1.NetworkTypeUnderlay {
			// check if this node belongs to the subnet
			inSubnet := false
			for _, n := range network.Status.NodeList {
				if n == c.config.NodeName {
					inSubnet = true
					break
				}
			}

			if !inSubnet {
				klog.Infof("Ignore reconciling underlay subnet %v", subnet.Name)
				continue
			}
		}

		// if this node belongs to the subnet
		// ensure bridge interface here
		netID := subnet.Spec.NetID
		if netID == nil {
			netID = network.Spec.NetID
		}

		subnetCidr, err := netlink.ParseIPNet(subnet.Spec.Range.CIDR)
		if err != nil {
			return fmt.Errorf("failed to parse subnet cidr %v error: %v", subnet.Spec.Range.CIDR, err)
		}

		gatewayIP := net.ParseIP(subnet.Spec.Range.Gateway)
		if gatewayIP == nil {
			return fmt.Errorf("invalid gateway ip %v", subnet.Spec.Range.Gateway)
		}

		var forwardNodeIfName string
		var autoNatOutgoing, isOverlay bool

		switch ramav1.GetNetworkType(network) {
		case ramav1.NetworkTypeUnderlay:
			forwardNodeIfName, err = containernetwork.EnsureVlanIf(c.config.NodeVlanIfName, netID)
			if err != nil {
				return fmt.Errorf("ensure vlan forward node if failed: %v", err)
			}
		case ramav1.NetworkTypeOverlay:
			forwardNodeIfName, err = containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, netID)
			if err != nil {
				return fmt.Errorf("generate vxlan forward node if name failed: %v", err)
			}
			isOverlay = true
			autoNatOutgoing = ramav1.IsSubnetAutoNatOutgoing(&subnet.Spec)
		}

		// create policy route
		routeManager := c.getRouterManager(subnet.Spec.Range.Version)
		routeManager.AddSubnetInfo(subnetCidr, gatewayIP, forwardNodeIfName, autoNatOutgoing, isOverlay)
	}

	if err := c.routeV4Manager.SyncRoutes(); err != nil {
		return fmt.Errorf("sync ipv4 routes failed: %v", err)
	}

	if err := c.routeV6Manager.SyncRoutes(); err != nil {
		return fmt.Errorf("sync ipv6 routes failed: %v", err)
	}

	c.iptablesSyncTrigger()

	return nil
}
