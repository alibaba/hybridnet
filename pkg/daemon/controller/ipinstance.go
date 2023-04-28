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
	"net"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
)

type ipInstanceReconciler struct {
	client.Client
	ctrlHubRef *CtrlHub
}

func (r *ipInstanceReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	start := time.Now()
	defer func() {
		endTime := time.Since(start)
		logger.V(2).Info("IPInstance information reconciled", "time", endTime)
	}()

	ipInstanceList := &networkingv1.IPInstanceList{}
	if err := r.List(ctx, ipInstanceList,
		client.MatchingLabels{constants.LabelNode: r.ctrlHubRef.config.NodeName}); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("list ip instances for node %v error: %v",
			r.ctrlHubRef.config.NodeName, err)
	}

	r.ctrlHubRef.neighV4Manager.ResetInfos()
	r.ctrlHubRef.neighV6Manager.ResetInfos()

	r.ctrlHubRef.addrV4Manager.ResetInfos()
	r.ctrlHubRef.bgpManager.ResetIPInfos()

	overlayForwardNodeIfName, _, _, err := collectGlobalNetworkInfoAndInit(ctx, r,
		r.ctrlHubRef.config.NodeVxlanIfName, r.ctrlHubRef.config.NodeName, r.ctrlHubRef.bgpManager, false)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to collect global network info and init: %v", err)
	}

	for _, ipInstance := range ipInstanceList.Items {
		// skip reserved ip instance
		if networkingv1.IsReserved(&ipInstance) {
			continue
		}

		netID := ipInstance.Spec.Address.NetID
		podIP, subnetCidr, err := net.ParseCIDR(ipInstance.Spec.Address.IP)
		if err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("parse pod ip %v error: %v", ipInstance.Spec.Address.IP, err)
		}

		network := &networkingv1.Network{}
		if err := r.Get(ctx, types.NamespacedName{Name: ipInstance.Spec.Network}, network); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get network for ip instance %v: %v",
				ipInstance.Name, err)
		}

		var forwardNodeIfName string
		switch networkingv1.GetNetworkMode(network) {
		case networkingv1.NetworkModeVlan:
			forwardNodeIfName, err = daemonutils.GenerateVlanNetIfName(r.ctrlHubRef.config.NodeVlanIfName, netID)
			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to generate vlan forward node interface name: %v", err)
			}

			if ipInstance.Spec.Address.Version == networkingv1.IPv4 {
				// if vlan arp enhancement is not enabled, all the enhanced address will be cleaned
				if r.ctrlHubRef.config.EnableVlanArpEnhancement {
					r.ctrlHubRef.addrV4Manager.TryAddPodInfo(forwardNodeIfName, subnetCidr, podIP)
				}
			}
		case networkingv1.NetworkModeVxlan:
			forwardNodeIfName, err = daemonutils.GenerateVxlanNetIfName(r.ctrlHubRef.config.NodeVxlanIfName, netID)
			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to generate vxlan forward node interface name: %v", err)
			}
		case networkingv1.NetworkModeBGP:
			r.ctrlHubRef.bgpManager.RecordIP(podIP, false)
		case networkingv1.NetworkModeGlobalBGP:
			r.ctrlHubRef.bgpManager.RecordIP(podIP, true)
		}

		// create proxy neigh
		neighManager := r.ctrlHubRef.getNeighManager(ipInstance.Spec.Address.Version)

		if len(overlayForwardNodeIfName) != 0 {
			// Every underlay pod should also add a proxy neigh on overlay forward interface.
			// neighManager.AddPodInfo is idempotent
			neighManager.AddPodInfo(podIP, overlayForwardNodeIfName)
		}

		// don't need to create proxy neigh for a bgp ip instance
		if len(forwardNodeIfName) != 0 {
			neighManager.AddPodInfo(podIP, forwardNodeIfName)
		}
	}

	if err := r.ctrlHubRef.neighV4Manager.SyncNeighs(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync ipv4 neighs: %v", err)
	}

	globalDisabled, err := daemonutils.CheckIPv6GlobalDisabled()
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to check ipv6 global disabled: %v", err)
	}

	if !globalDisabled {
		if err := r.ctrlHubRef.neighV6Manager.SyncNeighs(); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync ipv6 neighs: %v", err)
		}
	}

	if err := r.ctrlHubRef.addrV4Manager.SyncAddresses(r.ctrlHubRef.getIPInstanceByAddress); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync ipv4 addresses: %v", err)
	}

	if err := r.ctrlHubRef.bgpManager.SyncIPInfos(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync bgp ip paths: %v", err)
	}

	r.ctrlHubRef.iptablesSyncTrigger()

	return reconcile.Result{}, nil
}

func (r *ipInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ipInstanceController, err := controller.New("ip-instance", mgr, controller.Options{
		Reconciler:   r,
		RecoverPanic: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create ip instance controller: %v", err)
	}

	if err := ipInstanceController.Watch(&source.Kind{Type: &networkingv1.IPInstance{}},
		&fixedKeyHandler{key: "ForIPInstanceChange"},
		&predicate.ResourceVersionChangedPredicate{},
		&predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				ipInstance := createEvent.Object.(*networkingv1.IPInstance)
				return ipInstance.GetLabels()[constants.LabelNode] == r.ctrlHubRef.config.NodeName
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				ipInstance := deleteEvent.Object.(*networkingv1.IPInstance)
				return ipInstance.GetLabels()[constants.LabelNode] == r.ctrlHubRef.config.NodeName
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldIPInstance := updateEvent.ObjectOld.(*networkingv1.IPInstance)
				newIPInstance := updateEvent.ObjectNew.(*networkingv1.IPInstance)

				if newIPInstance.GetLabels()[constants.LabelNode] == r.ctrlHubRef.config.NodeName ||
					oldIPInstance.GetLabels()[constants.LabelNode] == r.ctrlHubRef.config.NodeName {
					return true
				}

				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				ipInstance := genericEvent.Object.(*networkingv1.IPInstance)
				return ipInstance.GetLabels()[constants.LabelNode] == r.ctrlHubRef.config.NodeName
			},
		}); err != nil {
		return fmt.Errorf("failed to watch networkingv1.IPInstance for ip instance controller: %v", err)
	}

	if err := ipInstanceController.Watch(r.ctrlHubRef.ipInstanceTriggerSourceForHostLink, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch ipInstanceTriggerSourceForHostLink for ip instance controller: %v", err)
	}

	return nil
}
