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
	"sort"

	utils2 "github.com/alibaba/hybridnet/pkg/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"golang.org/x/sys/unix"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"

	"github.com/alibaba/hybridnet/pkg/daemon/utils"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/daemon/vxlan"
	"github.com/alibaba/hybridnet/pkg/feature"
	ipamutils "github.com/alibaba/hybridnet/pkg/ipam/utils"

	"github.com/vishvananda/netlink"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const nodeKind = "Node"

type nodeInfoReconciler struct {
	client.Client
	ctrlHubRef *CtrlHub
}

func (r *nodeInfoReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling node information")

	var overlayNetID *int32
	var overlayNodeNum int

	networkList := &networkingv1.NetworkList{}
	if err := r.List(ctx, networkList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list network %v", err)
	}

	for _, network := range networkList.Items {
		if networkingv1.GetNetworkType(&network) == networkingv1.NetworkTypeOverlay {
			overlayNetID = network.Spec.NetID
			overlayNodeNum = len(network.Status.NodeList)
			break
		}
	}

	// overlay network not exist, do nothing
	if overlayNetID == nil {
		return reconcile.Result{}, nil
	}

	vtepIP, vtepMac, err := r.selectVtepAddressFromLink()
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to select vtep address: %v", err)
	}

	vxlanLinkName, err := utils.GenerateVxlanNetIfName(r.ctrlHubRef.config.NodeVxlanIfName, overlayNetID)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to generate vxlan interface name: %v", err)
	}

	// Node objects are not supposed to be in list/watch cache.
	thisNode := &corev1.Node{}
	if err := r.ctrlHubRef.mgr.GetAPIReader().Get(ctx, types.NamespacedName{
		Name: r.ctrlHubRef.config.NodeName,
	}, thisNode); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get node object %v: %v",
			r.ctrlHubRef.config.NodeName, err)
	}

	nodeLocalVxlanAddrs, err := r.selectNodeLocalVxlanAddrs(thisNode, vtepIP, vxlanLinkName)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to select node local vxlan addresses: %v", err)
	}
	if _, err := r.createOrUpdateNodeVxlanInfo(thisNode, vtepIP, vtepMac, nodeLocalVxlanAddrs); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to update node vxlan info: %v", err)
	}

	nodeInfoList := &networkingv1.NodeInfoList{}
	if err := r.List(ctx, nodeInfoList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed list node: %v", err)
	}

	// if the vtep ip change, vxlan interface will be rebuilt
	vxlanDev, err := vxlan.NewVxlanDevice(vxlanLinkName, int(*overlayNetID),
		r.ctrlHubRef.config.NodeVxlanIfName, vtepIP, r.ctrlHubRef.config.VxlanUDPPort,
		r.ctrlHubRef.config.VxlanBaseReachableTime, true)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to create vxlan device %v: %v", vxlanLinkName, err)
	}

	if err := ensureVxlanInterfaceAddresses(vxlanDev, nodeLocalVxlanAddrs); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to ensure addresses for vxlan device %v: %v",
			vxlanLinkName, err)
	}

	for _, nodeInfo := range nodeInfoList.Items {
		if nodeInfo.Spec.VTEPInfo == nil ||
			len(nodeInfo.Spec.VTEPInfo.IP) == 0 ||
			len(nodeInfo.Spec.VTEPInfo.MAC) == 0 {
			// If node is not an overlay node, do nothing.
			continue
		}

		vtepMac, err := net.ParseMAC(nodeInfo.Spec.VTEPInfo.MAC)
		if err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse node vtep mac string %v: %v",
				nodeInfo.Spec.VTEPInfo.MAC, err)
		}

		vtepIP := net.ParseIP(nodeInfo.Spec.VTEPInfo.IP)
		if vtepIP == nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse node vtep ip string %v",
				nodeInfo.Spec.VTEPInfo.IP)
		}

		vxlanDev.RecordVtepInfo(vtepMac, vtepIP)
	}

	var remoteVtepList []*multiclusterv1.RemoteVtep

	if feature.MultiClusterEnabled() {
		remoteVtepList := &multiclusterv1.RemoteVtepList{}
		if err = r.List(ctx, remoteVtepList); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list remote vtep: %v", err)
		}

		for _, remoteVtep := range remoteVtepList.Items {
			vtepMac, err := net.ParseMAC(remoteVtep.Spec.VTEPInfo.MAC)
			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse remote vtep mac string %v: %v",
					remoteVtep.Spec.VTEPInfo.MAC, err)
			}

			vtepIP := net.ParseIP(remoteVtep.Spec.VTEPInfo.IP)
			if vtepIP == nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse remote vtep ip string %v",
					remoteVtep.Spec.VTEPInfo.IP)
			}

			vxlanDev.RecordVtepInfo(vtepMac, vtepIP)
		}
	}

	if err := r.ctrlHubRef.nodeIPCache.UpdateNodeIPs(nodeInfoList.Items, r.ctrlHubRef.config.NodeName,
		remoteVtepList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to update node ip cache: %v", err)
	}

	// Only delete fdb when the number of NodeInfo objects equals the number of overlay Nodes, to avoid network flapping.
	if err := vxlanDev.SyncVtepInfo(len(nodeInfoList.Items) == overlayNodeNum); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync vtep info for vxlan device %v: %v",
			vxlanDev.Link().Name, err)
	}

	if len(nodeInfoList.Items) != overlayNodeNum {
		logger.Info("The number of NodeInfo objects are not equal to overlay nodes, "+
			"vxlan fdb delete operation will not be executed",
			"NodeInfo num", len(nodeInfoList.Items),
			"Overlay Node num", overlayNodeNum)
	}

	r.ctrlHubRef.iptablesSyncTrigger()

	// Vxlan device might be regenerated, if that happens, all the related routes will be cleaned.
	// So subnet controller need to be triggered again.
	r.ctrlHubRef.subnetControllerTriggerSource.Trigger()

	return reconcile.Result{}, nil
}

func (r *nodeInfoReconciler) selectVtepAddressFromLink() (net.IP, net.HardwareAddr, error) {
	link, err := netlink.LinkByName(r.ctrlHubRef.config.NodeVxlanIfName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node vxlan interface %v: %v",
			r.ctrlHubRef.config.NodeVxlanIfName, err)
	}

	// Use parent's valid ipv4 address first, try ipv6 address if no valid ipv4 address exist.
	existParentAddrList, err := utils.ListAllGlobalUnicastAddress(link)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list address for vxlan parent link %v: %v",
			link.Attrs().Name, err)
	}

	if len(existParentAddrList) == 0 {
		return nil, nil, fmt.Errorf("there is no available ip for vxlan parent link %v",
			link.Attrs().Name)
	}

	var vtepIP net.IP
existParentAddrLoop:
	for _, addr := range existParentAddrList {
		for _, cidr := range r.ctrlHubRef.config.VtepAddressCIDRs {
			if cidr.Contains(addr.IP) {
				vtepIP = addr.IP
				break existParentAddrLoop
			}
		}
	}

	if vtepIP == nil {
		return nil, nil, fmt.Errorf("no availuable vtep ip can be used for link %v",
			link.Attrs().Name)
	}

	return vtepIP, link.Attrs().HardwareAddr, nil
}

func (r *nodeInfoReconciler) selectNodeLocalVxlanAddrs(thisNode *corev1.Node, vtepIP net.IP,
	vxlanLinkName string) ([]netlink.Addr, error) {
	existAllAddrList, err := containernetwork.ListLocalAddressExceptLink(vxlanLinkName)
	if err != nil {
		return nil, fmt.Errorf("failed to list address for all interfaces: %v", err)
	}

	var nodeLocalVxlanAddr []netlink.Addr
	for _, addr := range existAllAddrList {
		// Add vtep ip and node object ip by default.
		isNodeObjectAddr := false
		for _, nodeObjectAddr := range thisNode.Status.Addresses {
			if nodeObjectAddr.Address == addr.IP.String() {
				isNodeObjectAddr = true
				break
			}
		}

		if isNodeObjectAddr || addr.IP.Equal(vtepIP) {
			nodeLocalVxlanAddr = append(nodeLocalVxlanAddr, addr)
			continue
		}

		// Add extra node local vxlan ip.
		for _, cidr := range r.ctrlHubRef.config.ExtraNodeLocalVxlanIPCidrs {
			if cidr.Contains(addr.IP) {
				nodeLocalVxlanAddr = append(nodeLocalVxlanAddr, addr)
			}
		}
	}

	return nodeLocalVxlanAddr, nil
}

func ensureVxlanInterfaceAddresses(vxlanDev *vxlan.Device, addresses []netlink.Addr) error {
	nodeLocalVxlanAddrMap := map[string]bool{}
	for _, addr := range addresses {
		nodeLocalVxlanAddrMap[addr.IP.String()] = true
	}

	vxlanDevAddrList, err := utils.ListAllGlobalUnicastAddress(vxlanDev.Link())
	if err != nil {
		return fmt.Errorf("failed to list address for vxlan interface %v: %v",
			vxlanDev.Link().Name, err)
	}

	existVxlanDevAddrMap := map[string]bool{}
	for _, addr := range vxlanDevAddrList {
		existVxlanDevAddrMap[addr.IP.String()] = true
	}

	// Add all node local vxlan ip address to vxlan interface.
	for _, addr := range addresses {
		if _, exist := existVxlanDevAddrMap[addr.IP.String()]; !exist {
			if err := netlink.AddrAdd(vxlanDev.Link(), &netlink.Addr{
				IPNet: addr.IPNet,
				Label: "",
				Flags: unix.IFA_F_NOPREFIXROUTE,
			}); err != nil {
				return fmt.Errorf("failed to set addr %v to link %v: %v",
					addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	// Delete invalid address.
	for _, addr := range vxlanDevAddrList {
		if _, exist := nodeLocalVxlanAddrMap[addr.IP.String()]; !exist {
			if err := netlink.AddrDel(vxlanDev.Link(), &addr); err != nil {
				return fmt.Errorf("failed to del addr %v for link %v: %v",
					addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	return nil
}

// createOrUpdateIPInstance will create or update an NodeInfo
func (r *nodeInfoReconciler) createOrUpdateNodeVxlanInfo(thisNode *corev1.Node,
	vtepIP net.IP, vtepMac net.HardwareAddr, nodeLocalVxlanAddr []netlink.Addr) (info *networkingv1.NodeInfo, err error) {
	var nodeInfo = &networkingv1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.ctrlHubRef.config.NodeName,
		},
	}

	operationResult, err := controllerutil.CreateOrPatch(context.TODO(), r, nodeInfo, func() error {
		if !nodeInfo.DeletionTimestamp.IsZero() {
			return fmt.Errorf("node info %s is deleting, can not be updated", nodeInfo.Name)
		}

		localIPs := utils.GenerateIPStringList(nodeLocalVxlanAddr)
		sort.Strings(localIPs)

		nodeInfo.Spec.VTEPInfo = &networkingv1.VTEPInfo{
			IP:       vtepIP.String(),
			MAC:      vtepMac.String(),
			LocalIPs: localIPs,
		}

		nodeInfo.OwnerReferences = []metav1.OwnerReference{
			*ipamutils.NewControllerRef(thisNode, corev1.SchemeGroupVersion.WithKind(nodeKind),
				true, false),
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create or patch node info %v:%v", nodeInfo.Name, err)
	}

	if operationResult != controllerutil.OperationResultNone {
		var updateTimestamp string
		updateTimestamp, err = metav1.Now().MarshalQueryParameter()
		if err != nil {
			return nil, fmt.Errorf("failed to generate update timestamp: %v", err)
		}

		if err = r.Status().Patch(context.TODO(), nodeInfo,
			client.RawPatch(types.MergePatchType,
				[]byte(fmt.Sprintf(`{"status":{"updateTimestamp":%q}}`, updateTimestamp)))); err != nil {
			return nil, fmt.Errorf("failed to update node info status for %v: %v", nodeInfo.Name, err)
		}
	}

	return nodeInfo, err
}

func (r *nodeInfoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	nodeController, err := controller.New("node", mgr, controller.Options{
		Reconciler:   r,
		RecoverPanic: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create node controller: %v", err)
	}

	if err := nodeController.Watch(&source.Kind{Type: &networkingv1.NodeInfo{}},
		&fixedKeyHandler{key: ActionReconcileNodeInfo},
		&predicate.GenerationChangedPredicate{},
	); err != nil {
		return fmt.Errorf("failed to watch corev1.Node for node controller: %v", err)
	}

	if err := nodeController.Watch(&source.Kind{Type: &networkingv1.Network{}},
		&fixedKeyHandler{key: ActionReconcileNodeInfo},
		predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldNetwork := updateEvent.ObjectOld.(*networkingv1.Network)
				newNetwork := updateEvent.ObjectNew.(*networkingv1.Network)
				return !utils2.DeepEqualStringSlice(oldNetwork.Status.NodeList, newNetwork.Status.NodeList)
			},
			CreateFunc: func(createEvent event.CreateEvent) bool {
				network := createEvent.Object.(*networkingv1.Network)
				return networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				network := deleteEvent.Object.(*networkingv1.Network)
				return networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		},
	); err != nil {
		return fmt.Errorf("failed to watch networkingv1.Network for node controller: %v", err)
	}

	if err := nodeController.Watch(r.ctrlHubRef.nodeControllerTriggerSource, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch nodeControllerTriggerSource for node controller: %v", err)
	}

	if feature.MultiClusterEnabled() {
		if err := nodeController.Watch(&source.Kind{Type: &multiclusterv1.RemoteVtep{}},
			&fixedKeyHandler{key: ActionReconcileNodeInfo},
			predicate.Funcs{
				UpdateFunc: func(updateEvent event.UpdateEvent) bool {
					oldRemoteVtep := updateEvent.ObjectOld.(*multiclusterv1.RemoteVtep)
					newRemoteVtep := updateEvent.ObjectNew.(*multiclusterv1.RemoteVtep)

					if oldRemoteVtep.Spec.VTEPInfo.IP != newRemoteVtep.Spec.VTEPInfo.IP ||
						oldRemoteVtep.Spec.VTEPInfo.MAC != newRemoteVtep.Spec.VTEPInfo.MAC ||
						!utils2.DeepEqualStringSlice(oldRemoteVtep.Spec.VTEPInfo.LocalIPs, newRemoteVtep.Spec.LocalIPs) ||
						!isIPListEqual(oldRemoteVtep.Spec.EndpointIPList, newRemoteVtep.Spec.EndpointIPList) {
						return true
					}
					return false
				},
			},
		); err != nil {
			return fmt.Errorf("failed to watch multiclusterv1.RemoteVtep for node controller: %v", err)
		}
	}

	return nil
}
