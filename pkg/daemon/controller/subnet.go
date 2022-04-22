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
	"reflect"

	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/alibaba/hybridnet/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/feature"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type subnetReconciler struct {
	client.Client
	ctrlHubRef *CtrlHub
}

func (r *subnetReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling subnet information")

	subnetList := &networkingv1.SubnetList{}
	if err := r.List(ctx, subnetList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list subnet %v", err)
	}

	r.ctrlHubRef.routeV4Manager.ResetInfos()
	r.ctrlHubRef.routeV6Manager.ResetInfos()

	r.ctrlHubRef.bgpManager.ResetPeerAndSubnetInfos()

	// only update bgp peer info in subnet reconcile
	overlayForwardNodeIfName, bgpGatewayIP, err := collectGlobalNetworkInfoAndInit(ctx, r,
		r.ctrlHubRef.config.NodeVxlanIfName, r.ctrlHubRef.config.NodeName, r.ctrlHubRef.bgpManager, true)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to collect global network info and init: %v", err)
	}

	for _, subnet := range subnetList.Items {
		network := &networkingv1.Network{}
		if err := r.Get(ctx, types.NamespacedName{Name: subnet.Spec.Network}, network); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get network for subnet %v", subnet.Name)
		}

		isUnderlayOnHost := nodeBelongsToNetwork(r.ctrlHubRef.config.NodeName, network)

		// if this node belongs to the subnet
		// ensure bridge interface here
		netID := subnet.Spec.NetID
		if netID == nil {
			netID = network.Spec.NetID
		}

		subnetCidr, gatewayIP, startIP, endIP, excludeIPs, _, err := parseSubnetSpecRangeMeta(&subnet.Spec.Range)
		if err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse subnet %v spec range meta: %v", subnet.Name, err)
		}

		var forwardNodeIfName string
		var autoNatOutgoing, isOverlay bool
		networkMode := networkingv1.GetNetworkMode(network)

		switch networkMode {
		case networkingv1.NetworkModeVlan:
			if isUnderlayOnHost {
				forwardNodeIfName, err = daemonutils.EnsureVlanIf(r.ctrlHubRef.config.NodeVlanIfName, netID)
				if err != nil {
					return reconcile.Result{Requeue: true}, fmt.Errorf("failed to ensure vlan forward node interface: %v", err)
				}
			}
		case networkingv1.NetworkModeVxlan:
			forwardNodeIfName = overlayForwardNodeIfName
			isOverlay = true
			autoNatOutgoing = networkingv1.IsSubnetAutoNatOutgoing(&subnet.Spec)
		case networkingv1.NetworkModeBGP:
			if isUnderlayOnHost {
				forwardNodeIfName = r.ctrlHubRef.config.NodeBGPIfName
				r.ctrlHubRef.bgpManager.RecordSubnet(subnetCidr)
				// use peer ip as gateway
				gatewayIP = bgpGatewayIP
			}
		case networkingv1.NetworkModeGlobalBGP:
			if bgpGatewayIP == nil {
				// node does not belong to any underlay bgp network
				isUnderlayOnHost = false
			} else {
				isUnderlayOnHost = true
				forwardNodeIfName = r.ctrlHubRef.config.NodeBGPIfName

				// don't need to record subnet for bgp manager
				gatewayIP = bgpGatewayIP
			}
		default:
			return reconcile.Result{Requeue: true}, fmt.Errorf("invalic network mode %v for %v", networkMode, network.Name)
		}

		// create policy route
		routeManager := r.ctrlHubRef.getRouterManager(subnet.Spec.Range.Version)
		routeManager.AddSubnetInfo(subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
			forwardNodeIfName, autoNatOutgoing, isOverlay, isUnderlayOnHost, networkMode)
	}

	if feature.MultiClusterEnabled() {
		logger.Info("Reconciling remote subnet information")

		remoteSubnetList := &multiclusterv1.RemoteSubnetList{}
		if err := r.List(ctx, remoteSubnetList); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list remote subnet %v", err)
		}

		for _, remoteSubnet := range remoteSubnetList.Items {
			subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
				_, err := parseSubnetSpecRangeMeta(&remoteSubnet.Spec.Range)

			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse subnet %v spec range meta: %v", remoteSubnet.Name, err)
			}

			var isOverlay = multiclusterv1.GetRemoteSubnetType(&remoteSubnet) == networkingv1.NetworkTypeOverlay

			routeManager := r.ctrlHubRef.getRouterManager(remoteSubnet.Spec.Range.Version)
			err = routeManager.AddRemoteSubnetInfo(subnetCidr, gatewayIP, startIP, endIP, excludeIPs, isOverlay)

			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to add remote subnet info: %v", err)
			}
		}
	}

	if err := r.ctrlHubRef.routeV4Manager.SyncRoutes(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync ipv4 routes: %v", err)
	}

	globalDisabled, err := daemonutils.CheckIPv6GlobalDisabled()
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to check ipv6 global disabled: %v", err)
	}

	if !globalDisabled {
		if err := r.ctrlHubRef.routeV6Manager.SyncRoutes(); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync ipv6 routes: %v", err)
		}
	}

	if err := r.ctrlHubRef.bgpManager.SyncPeerAndSubnetInfos(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync bgp peers and subnet paths: %v", err)
	}

	r.ctrlHubRef.iptablesSyncTrigger()

	return reconcile.Result{}, nil
}

func (r *subnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	subnetController, err := controller.New("subnet", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to create subnet controller: %v", err)
	}

	if err := subnetController.Watch(&source.Kind{Type: &networkingv1.Subnet{}},
		&fixedKeyHandler{key: ActionReconcileSubnet},
		&predicate.ResourceVersionChangedPredicate{},
		&predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldSubnet := updateEvent.ObjectOld.(*networkingv1.Subnet)
				newSubnet := updateEvent.ObjectNew.(*networkingv1.Subnet)

				oldSubnetNetID := oldSubnet.Spec.NetID
				newSubnetNetID := newSubnet.Spec.NetID

				if (oldSubnetNetID == nil && newSubnetNetID != nil) ||
					(oldSubnetNetID != nil && newSubnetNetID == nil) ||
					(oldSubnetNetID != nil && newSubnetNetID != nil && *oldSubnetNetID != *newSubnetNetID) ||
					oldSubnet.Spec.Network != newSubnet.Spec.Network ||
					!reflect.DeepEqual(oldSubnet.Spec.Range, newSubnet.Spec.Range) ||
					networkingv1.IsSubnetAutoNatOutgoing(&oldSubnet.Spec) != networkingv1.IsSubnetAutoNatOutgoing(&newSubnet.Spec) {
					return true
				}
				return false
			},
		}); err != nil {
		return fmt.Errorf("failed to watch networkingv1.Subnet for subnet controller: %v", err)
	}

	if err := subnetController.Watch(&source.Kind{Type: &networkingv1.Network{}},
		&fixedKeyHandler{key: ActionReconcileSubnet},
		&predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldNetwork := updateEvent.ObjectOld.(*networkingv1.Network)
				newNetwork := updateEvent.ObjectNew.(*networkingv1.Network)

				if utils.DeepEqualStringSlice(oldNetwork.Status.SubnetList, newNetwork.Status.SubnetList) ||
					utils.DeepEqualStringSlice(oldNetwork.Status.NodeList, newNetwork.Status.NodeList) {
					return true
				}

				if !reflect.DeepEqual(oldNetwork.Spec.Config, newNetwork.Spec.Config) {
					return true
				}

				return false
			},
		},
	); err != nil {
		return fmt.Errorf("failed to watch networkingv1.Network for subnet controller: %v", err)
	}

	if err := subnetController.Watch(&source.Kind{Type: &corev1.Node{}},
		&fixedKeyHandler{key: ActionReconcileSubnet},
		predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return checkNodeUpdate(updateEvent)
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		}); err != nil {
		return fmt.Errorf("failed to watch corev1.Node for subnet controller: %v", err)
	}

	if err := subnetController.Watch(r.ctrlHubRef.subnetControllerTriggerSource, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch subnetControllerTriggerSource for subnet controller: %v", err)
	}

	// enable multicluster feature
	if feature.MultiClusterEnabled() {
		if err := subnetController.Watch(&source.Kind{
			Type: &multiclusterv1.RemoteSubnet{}},
			&fixedKeyHandler{key: ActionReconcileSubnet},
			predicate.Funcs{
				UpdateFunc: func(updateEvent event.UpdateEvent) bool {
					oldRs := updateEvent.ObjectOld.(*multiclusterv1.RemoteSubnet)
					newRs := updateEvent.ObjectNew.(*multiclusterv1.RemoteSubnet)

					if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
						!reflect.DeepEqual(oldRs.Spec.Range, newRs.Spec.Range) ||
						multiclusterv1.GetRemoteSubnetType(oldRs) != multiclusterv1.GetRemoteSubnetType(newRs) {
						return true
					}
					return false
				},
			},
		); err != nil {
			return fmt.Errorf("failed to watch multiclusterv1.RemoteSubnet for subnet controller: %v", err)
		}
	}

	return nil
}
