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
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/vxlan"
	"github.com/alibaba/hybridnet/pkg/feature"

	"github.com/vishvananda/netlink"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type nodeReconciler struct {
	client.Client
	ctrlHubRef *CtrlHub
}

func (r *nodeReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling node information")

	networkList := &networkingv1.NetworkList{}
	if err := r.List(ctx, networkList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list network %v", err)
	}

	var overlayNetID *int32
	for _, network := range networkList.Items {
		if networkingv1.GetNetworkType(&network) == networkingv1.NetworkTypeOverlay {
			overlayNetID = network.Spec.NetID
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

	thisNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.ctrlHubRef.config.NodeName}, thisNode); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get node %v object: %v",
			r.ctrlHubRef.config.NodeName, err)
	}

	nodeLocalVxlanAddrs, err := r.selectNodeLocalVxlanAddrs(thisNode, vtepIP, vxlanLinkName)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to select node local vxlan addresses: %v", err)
	}

	if err := r.updateNodeVxlanAnnotations(thisNode, vtepIP, vtepMac, nodeLocalVxlanAddrs); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to update node vxlan annotations: %v", err)
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
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

	var incompleteVtepInfoNodes []string
	for _, node := range nodeList.Items {
		if node.Annotations[constants.AnnotationNodeVtepMac] == "" ||
			node.Annotations[constants.AnnotationNodeVtepIP] == "" ||
			node.Annotations[constants.AnnotationNodeLocalVxlanIPList] == "" {
			incompleteVtepInfoNodes = append(incompleteVtepInfoNodes, node.Name)
			continue
		}

		vtepMac, err := net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
		if err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse node vtep mac string %v: %v",
				node.Annotations[constants.AnnotationNodeVtepMac], err)
		}

		vtepIP := net.ParseIP(node.Annotations[constants.AnnotationNodeVtepIP])
		if vtepIP == nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to parse node vtep ip string %v",
				node.Annotations[constants.AnnotationNodeVtepIP])
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

	if err := r.ctrlHubRef.nodeIPCache.UpdateNodeIPs(nodeList.Items, r.ctrlHubRef.config.NodeName, remoteVtepList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to update node ip cache: %v", err)
	}

	if err := vxlanDev.SyncVtepInfo(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to sync vtep info for vxlan device %v: %v",
			vxlanDev.Link().Name, err)
	}

	r.ctrlHubRef.iptablesSyncTrigger()

	// Vxlan device might be regenerated, if that happens, all the related routes will be cleaned.
	// So subnet controller need to be triggered again.
	r.ctrlHubRef.subnetControllerTriggerSource.Trigger()

	if len(incompleteVtepInfoNodes) != 0 {
		return reconcile.Result{Requeue: true}, fmt.Errorf("some of the nodes' vtep information has not been updated, "+
			"error nodes are: %v", incompleteVtepInfoNodes)
	}

	return reconcile.Result{}, nil
}

func (r *nodeReconciler) selectVtepAddressFromLink() (net.IP, net.HardwareAddr, error) {
	link, err := netlink.LinkByName(r.ctrlHubRef.config.NodeVxlanIfName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node vxlan interface %v: %v",
			r.ctrlHubRef.config.NodeVxlanIfName, err)
	}

	// Use parent's valid ipv4 address first, try ipv6 address if no valid ipv4 address exist.
	existParentAddrList, err := utils.ListAllAddress(link)
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

func (r *nodeReconciler) selectNodeLocalVxlanAddrs(thisNode *corev1.Node, vtepIP net.IP,
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

func (r *nodeReconciler) updateNodeVxlanAnnotations(thisNode *corev1.Node,
	vtepIP net.IP, vtepMac net.HardwareAddr, nodeLocalVxlanAddr []netlink.Addr) error {
	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s","%s":"%s","%s":"%s"}}}`,
		constants.AnnotationNodeVtepIP, vtepIP.String(),
		constants.AnnotationNodeVtepMac, vtepMac.String(),
		constants.AnnotationNodeLocalVxlanIPList, utils.GenerateIPListString(nodeLocalVxlanAddr))

	if err := r.ctrlHubRef.mgr.GetClient().Patch(context.TODO(),
		thisNode, client.RawPatch(types.StrategicMergePatchType, []byte(patchData))); err != nil {
		return err
	}

	return nil
}

func ensureVxlanInterfaceAddresses(vxlanDev *vxlan.Device, addresses []netlink.Addr) error {
	nodeLocalVxlanAddrMap := map[string]bool{}
	for _, addr := range addresses {
		nodeLocalVxlanAddrMap[addr.IP.String()] = true
	}

	vxlanDevAddrList, err := utils.ListAllAddress(vxlanDev.Link())
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

func checkNodeUpdate(updateEvent event.UpdateEvent) bool {
	oldNode := updateEvent.ObjectOld.(*corev1.Node)
	newNode := updateEvent.ObjectNew.(*corev1.Node)

	if oldNode.Annotations[constants.AnnotationNodeVtepIP] != newNode.Annotations[constants.AnnotationNodeVtepIP] ||
		oldNode.Annotations[constants.AnnotationNodeVtepMac] != newNode.Annotations[constants.AnnotationNodeVtepMac] ||
		oldNode.Annotations[constants.AnnotationNodeLocalVxlanIPList] != newNode.Annotations[constants.AnnotationNodeLocalVxlanIPList] {
		return true
	}
	return false
}

func (r *nodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	nodeController, err := controller.New("node", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to create node controller: %v", err)
	}

	if err := nodeController.Watch(&source.Kind{Type: &corev1.Node{}},
		&fixedKeyHandler{key: ActionReconcileNode},
		&predicate.ResourceVersionChangedPredicate{},
		&predicate.AnnotationChangedPredicate{},
		&predicate.Funcs{
			UpdateFunc: checkNodeUpdate,
		},
	); err != nil {
		return fmt.Errorf("failed to watch corev1.Node for node controller: %v", err)
	}

	if err := nodeController.Watch(&source.Kind{Type: &networkingv1.Network{}},
		&fixedKeyHandler{key: ActionReconcileNode},
		predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return false
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
			&fixedKeyHandler{key: ActionReconcileNode},
			predicate.Funcs{
				UpdateFunc: func(updateEvent event.UpdateEvent) bool {
					oldRemoteVtep := updateEvent.ObjectOld.(*multiclusterv1.RemoteVtep)
					newRemoteVtep := updateEvent.ObjectNew.(*multiclusterv1.RemoteVtep)

					if oldRemoteVtep.Spec.VTEPInfo.IP != newRemoteVtep.Spec.VTEPInfo.IP ||
						oldRemoteVtep.Spec.VTEPInfo.MAC != newRemoteVtep.Spec.VTEPInfo.MAC ||
						oldRemoteVtep.Annotations[constants.AnnotationNodeLocalVxlanIPList] != newRemoteVtep.Annotations[constants.AnnotationNodeLocalVxlanIPList] ||
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
