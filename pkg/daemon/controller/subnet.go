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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
	"github.com/alibaba/hybridnet/pkg/feature"

	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type subnetReconciler struct {
	client.Client
	controllerRef *Controller
	logger        logr.Logger
}

func (r *subnetReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	r.logger.Info("Reconciling subnet information")

	subnetList := &networkingv1.SubnetList{}
	if err := r.List(ctx, subnetList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list subnet %v", err)
	}

	r.controllerRef.routeV4Manager.ResetInfos()
	r.controllerRef.routeV6Manager.ResetInfos()

	for _, subnet := range subnetList.Items {
		network := &networkingv1.Network{}
		if err := r.Get(ctx, types.NamespacedName{Name: subnet.Spec.Network}, network); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get network for subnet %v", subnet.Name)
		}

		isUnderlayOnHost := false
		if networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeUnderlay {
			// check if this node belongs to the subnet
			for _, n := range network.Status.NodeList {
				if n == r.controllerRef.config.NodeName {
					isUnderlayOnHost = true
					break
				}
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
			return reconcile.Result{Requeue: true}, fmt.Errorf("parse subnet %v spec range meta failed: %v", subnet.Name, err)
		}

		var forwardNodeIfName string
		var autoNatOutgoing, isOverlay bool

		switch networkingv1.GetNetworkType(network) {
		case networkingv1.NetworkTypeUnderlay:
			if isUnderlayOnHost {
				forwardNodeIfName, err = containernetwork.EnsureVlanIf(r.controllerRef.config.NodeVlanIfName, netID)
				if err != nil {
					return reconcile.Result{Requeue: true}, fmt.Errorf("ensure vlan forward node if failed: %v", err)
				}
			}
		case networkingv1.NetworkTypeOverlay:
			forwardNodeIfName, err = containernetwork.GenerateVxlanNetIfName(r.controllerRef.config.NodeVxlanIfName, netID)
			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("generate vxlan forward node if name failed: %v", err)
			}
			isOverlay = true
			autoNatOutgoing = networkingv1.IsSubnetAutoNatOutgoing(&subnet.Spec)
		}

		// create policy route
		routeManager := r.controllerRef.getRouterManager(subnet.Spec.Range.Version)
		routeManager.AddSubnetInfo(subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
			forwardNodeIfName, autoNatOutgoing, isOverlay, isUnderlayOnHost)
	}

	if feature.MultiClusterEnabled() {
		klog.Info("Reconciling remote subnet information")

		remoteSubnetList := &networkingv1.RemoteSubnetList{}
		if err := r.List(ctx, remoteSubnetList); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list remote subnet %v", err)
		}

		for _, remoteSubnet := range remoteSubnetList.Items {
			subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
				_, err := parseSubnetSpecRangeMeta(&remoteSubnet.Spec.Range)

			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("parse subnet %v spec range meta failed: %v", remoteSubnet.Name, err)
			}

			var isOverlay = networkingv1.GetRemoteSubnetType(&remoteSubnet) == networkingv1.NetworkTypeOverlay

			routeManager := r.controllerRef.getRouterManager(remoteSubnet.Spec.Range.Version)
			err = routeManager.AddRemoteSubnetInfo(subnetCidr, gatewayIP, startIP, endIP, excludeIPs, isOverlay)

			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to add remote subnet info: %v", err)
			}
		}
	}

	if err := r.controllerRef.routeV4Manager.SyncRoutes(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("sync ipv4 routes failed: %v", err)
	}

	globalDisabled, err := containernetwork.CheckIPv6GlobalDisabled()
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("check ipv6 global disabled failed: %v", err)
	}

	if !globalDisabled {
		if err := r.controllerRef.routeV6Manager.SyncRoutes(); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("sync ipv6 routes failed: %v", err)
		}
	}

	r.controllerRef.iptablesSyncTrigger()

	return reconcile.Result{}, nil
}
