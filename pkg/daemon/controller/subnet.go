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
	"reflect"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	"github.com/oecp/rama/pkg/feature"

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
		!reflect.DeepEqual(oldSubnet.Spec.Range, newSubnet.Spec.Range) ||
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

		subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
			_, err := parseSubnetSpecRangeMeta(&subnet.Spec.Range)

		if err != nil {
			return fmt.Errorf("parse subnet %v spec range meta failed: %v", subnet.Name, err)
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
		routeManager.AddSubnetInfo(subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
			forwardNodeIfName, autoNatOutgoing, isOverlay)
	}

	if feature.MultiClusterEnabled() {
		klog.Info("Reconciling remote subnet information")

		remoteSubnetList, err := c.remoteSubnetLister.List(labels.Everything())
		if err != nil {
			return fmt.Errorf("failed to list remote subnet %v", err)
		}

		for _, remoteSubnet := range remoteSubnetList {
			subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
				_, err := parseSubnetSpecRangeMeta(&remoteSubnet.Spec.Range)

			if err != nil {
				return fmt.Errorf("parse subnet %v spec range meta failed: %v", remoteSubnet.Name, err)
			}

			var isOverlay = ramav1.GetRemoteSubnetType(remoteSubnet) == ramav1.NetworkTypeOverlay

			routeManager := c.getRouterManager(remoteSubnet.Spec.Range.Version)
			err = routeManager.AddRemoteSubnetInfo(subnetCidr, gatewayIP, startIP, endIP, excludeIPs, isOverlay)

			if err != nil {
				return fmt.Errorf("failed to add remote subnet info: %v", err)
			}
		}
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
