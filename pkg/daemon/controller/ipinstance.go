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
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
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

	networkList := &networkingv1.NetworkList{}
	if err := r.List(ctx, networkList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list network: %v", err)
	}

	var overlayExist bool
	var overlayForwardNodeIfName string
	var err error

	for _, network := range networkList.Items {
		switch networkingv1.GetNetworkMode(&network) {
		case networkingv1.NetworkModeVxlan:
			netID := network.Spec.NetID
			overlayForwardNodeIfName, err = daemonutils.GenerateVxlanNetIfName(r.ctrlHubRef.config.NodeVxlanIfName, netID)
			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to generate vxlan forward node if name: %v", err)
			}
			overlayExist = true
		case networkingv1.NetworkModeBGP:
			// check if this node belongs to the network, ignore it if not
			if !nodeBelongsToNetwork(r.ctrlHubRef.config.NodeName, &network) {
				continue
			}

			if network.Spec.NetID == nil {
				return reconcile.Result{Requeue: true},
					fmt.Errorf("the net id of network %v must to be set", network.Name)
			}

			localAS := uint32(*network.Spec.NetID)
			if err = r.ctrlHubRef.bgpManager.TryStart(localAS); err != nil {
				return reconcile.Result{Requeue: true},
					fmt.Errorf("try start bgp manager for network %v failed: %v", network.Name, err)
			}
		}
	}

	if !r.ctrlHubRef.upgradeWorkDone {
		if err := ensureExistPodConfigs(r.ctrlHubRef.config.LocalDirectTableNum, logger); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to ensure exist pod config: %v", err)
		}
		r.ctrlHubRef.upgradeWorkDone = true
	}

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

	for _, ipInstance := range ipInstanceList.Items {
		// skip reserved ip instance
		if networkingv1.IsReserved(&ipInstance) {
			continue
		}

		netID := ipInstance.Spec.Address.NetID
		if netID == nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("NetID of ip instance %v should not be nil", ipInstance.Name)
		}

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
			r.ctrlHubRef.bgpManager.RecordIP(podIP)
		}

		// create proxy neigh
		neighManager := r.ctrlHubRef.getNeighManager(ipInstance.Spec.Address.Version)

		if overlayExist {
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
	ipInstanceController, err := controller.New("ip-instance", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to create ip instance controller: %v", err)
	}

	if err := ipInstanceController.Watch(&source.Kind{Type: &networkingv1.IPInstance{}},
		&fixedKeyHandler{key: ActionReconcileIPInstance},
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

	if err := ipInstanceController.Watch(r.ctrlHubRef.ipInstanceControllerTriggerSource, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch ipInstanceControllerTriggerSource for ip instance controller: %v", err)
	}

	return nil
}

// TODO: update logic, need to be removed further
func ensureExistPodConfigs(localDirectTableNum int, logger logr.Logger) error {
	var netnsPaths []string
	var netnsDir string

	if daemonutils.ValidDockerNetnsDir(constants.DockerNetnsDir) {
		netnsDir = constants.DockerNetnsDir
	} else {
		logger.Info("docker netns path not exist, try containerd netns path",
			"docker-netns-path", constants.DockerNetnsDir,
			"containerd-netns-path", constants.ContainerdNetnsDir)
		netnsDir = constants.ContainerdNetnsDir
	}

	files, err := ioutil.ReadDir(netnsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, f := range files {
		if f.Name() == "default" {
			continue
		}
		fpath := netnsDir + "/" + f.Name()
		if daemonutils.IsProcFS(fpath) || daemonutils.IsNsFS(fpath) {
			netnsPaths = append(netnsPaths, fpath)
		}
	}

	logger.Info("load exist netns", "netns-path", netnsPaths)

	var hostLinkIndex int
	allocatedIPs := map[networkingv1.IPVersion]*daemonutils.IPInfo{}

	for _, netns := range netnsPaths {
		nsHandler, err := ns.GetNS(netns)
		if err != nil {
			return fmt.Errorf("get ns error: %v", err)
		}

		err = nsHandler.Do(func(netNS ns.NetNS) error {
			link, err := netlink.LinkByName(constants.ContainerNicName)
			if err != nil {
				return fmt.Errorf("get container interface error: %v", err)
			}

			v4Addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
			if err != nil {
				return fmt.Errorf("failed to get v4 container interface addr: %v", err)
			}

			var v4GatewayIP net.IP
			if len(v4Addrs) == 0 {
				allocatedIPs[networkingv1.IPv4] = nil
			} else {
				defaultRoute, err := daemonutils.GetDefaultRoute(netlink.FAMILY_V4)
				if err != nil {
					return fmt.Errorf("failed to get ipv4 default route: %v", err)
				}
				v4GatewayIP = defaultRoute.Gw
			}

			for _, addr := range v4Addrs {
				allocatedIPs[networkingv1.IPv4] = &daemonutils.IPInfo{
					Addr: addr.IP,
					Gw:   v4GatewayIP,
				}
			}

			v6Addrs, err := netlink.AddrList(link, netlink.FAMILY_V6)
			if err != nil {
				return fmt.Errorf("failed to get v6 container interface addr: %v", err)
			}

			var v6GatewayIP net.IP
			if len(v6Addrs) == 0 {
				allocatedIPs[networkingv1.IPv6] = nil
			} else {
				defaultRoute, err := daemonutils.GetDefaultRoute(netlink.FAMILY_V6)
				if err != nil {
					return fmt.Errorf("failed to get ipv6 default route: %v", err)
				}
				v6GatewayIP = defaultRoute.Gw
			}

			for _, addr := range v6Addrs {
				allocatedIPs[networkingv1.IPv6] = &daemonutils.IPInfo{
					Addr: addr.IP,
					Gw:   v6GatewayIP,
				}
			}

			_, hostLinkIndex, err = ip.GetVethPeerIfindex(constants.ContainerNicName)
			if err != nil {
				return fmt.Errorf("get host link index error: %v", err)
			}

			return nil
		})

		if err != nil {
			logger.Error(err, "get pod addresses and host link index error")
		}

		if hostLinkIndex == 0 {
			continue
		}

		hostLink, err := netlink.LinkByIndex(hostLinkIndex)
		if err != nil {
			return fmt.Errorf("failed to get host link by index %v: %v", hostLinkIndex, err)
		}

		// this container doesn't belong to k8s
		if !strings.HasSuffix(hostLink.Attrs().Name, "_h") {
			continue
		}

		if hostLink.Attrs().MasterIndex != 0 {
			bridge, err := netlink.LinkByIndex(hostLink.Attrs().MasterIndex)
			if err != nil {
				return fmt.Errorf("failed to get bridge by index %v: %v", hostLink.Attrs().MasterIndex, err)
			}

			if err := netlink.LinkDel(bridge); err != nil {
				return fmt.Errorf("failed to delete bridge %v: %v", bridge.Attrs().Name, err)
			}
		}

		if err := containernetwork.ConfigureHostNic(hostLink.Attrs().Name, allocatedIPs, localDirectTableNum); err != nil {
			return fmt.Errorf("failed to reconfigure host nic %v: %v", hostLink.Attrs().Name, err)
		}
	}

	return nil
}
