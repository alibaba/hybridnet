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

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	multiclusterv1 "github.com/alibaba/hybridnet/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
	"github.com/alibaba/hybridnet/pkg/daemon/vxlan"
	"github.com/alibaba/hybridnet/pkg/feature"

	"golang.org/x/sys/unix"

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

	link, err := netlink.LinkByName(r.ctrlHubRef.config.NodeVxlanIfName)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get node vxlan interface %v: %v",
			r.ctrlHubRef.config.NodeVxlanIfName, err)
	}

	// Use parent's valid ipv4 address first, try ipv6 address if no valid ipv4 address exist.
	existParentAddrList, err := containernetwork.ListAllAddress(link)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list address for vxlan parent link %v: %v",
			link.Attrs().Name, err)
	}

	if len(existParentAddrList) == 0 {
		return reconcile.Result{Requeue: true}, fmt.Errorf("there is no available ip for vxlan parent link %v",
			link.Attrs().Name)
	}

	var vtepIP net.IP
	for _, addr := range existParentAddrList {
		if addr.IP.IsGlobalUnicast() {
			vtepIP = addr.IP
			break
		}
	}

	thisNode := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.ctrlHubRef.config.NodeName}, thisNode); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to get this node %v object: %v",
			r.ctrlHubRef.config.NodeName, err)
	}

	preVtepIP, exist := thisNode.Annotations[constants.AnnotationNodeVtepIP]
	if exist {
		// if vtep ip has been set and still exist on parent interface
		for _, address := range existParentAddrList {
			if address.IP.String() == preVtepIP {
				vtepIP = address.IP
				break
			}
		}
	}

	if vtepIP == nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("no availuable vtep ip can be used for link %v",
			link.Attrs().Name)
	}
	vtepMac := link.Attrs().HardwareAddr

	vxlanLinkName, err := containernetwork.GenerateVxlanNetIfName(r.ctrlHubRef.config.NodeVxlanIfName, overlayNetID)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to generate vxlan interface name: %v", err)
	}

	existAllAddrList, err := containernetwork.ListLocalAddressExceptLink(vxlanLinkName)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list address for all interfaces: %v", err)
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

	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s","%s":"%s","%s":"%s"}}}`,
		constants.AnnotationNodeVtepIP, vtepIP.String(),
		constants.AnnotationNodeVtepMac, vtepMac.String(),
		constants.AnnotationNodeLocalVxlanIPList, containernetwork.GenerateIPListString(nodeLocalVxlanAddr))

	if err := r.ctrlHubRef.mgr.GetClient().Patch(context.TODO(),
		thisNode, client.RawPatch(types.StrategicMergePatchType, []byte(patchData))); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("update node label error: %v", err)
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed list node: %v", err)
	}

	vxlanDev, err := vxlan.NewVxlanDevice(vxlanLinkName, int(*overlayNetID),
		r.ctrlHubRef.config.NodeVxlanIfName, vtepIP, r.ctrlHubRef.config.VxlanUDPPort,
		r.ctrlHubRef.config.VxlanBaseReachableTime, true)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to create vxlan device %v: %v", vxlanLinkName, err)
	}

	nodeLocalVxlanAddrMap := map[string]bool{}
	for _, addr := range nodeLocalVxlanAddr {
		nodeLocalVxlanAddrMap[addr.IP.String()] = true
	}

	vxlanDevAddrList, err := containernetwork.ListAllAddress(vxlanDev.Link())
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list address for vxlan interface %v: %v",
			vxlanDev.Link().Name, err)
	}

	existVxlanDevAddrMap := map[string]bool{}
	for _, addr := range vxlanDevAddrList {
		existVxlanDevAddrMap[addr.IP.String()] = true
	}

	// Add all node local vxlan ip address to vxlan interface.
	for _, addr := range nodeLocalVxlanAddr {
		if _, exist := existVxlanDevAddrMap[addr.IP.String()]; !exist {
			if err := netlink.AddrAdd(vxlanDev.Link(), &netlink.Addr{
				IPNet: addr.IPNet,
				Label: "",
				Flags: unix.IFA_F_NOPREFIXROUTE,
			}); err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to set addr %v to link %v: %v",
					addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	// Delete invalid address.
	for _, addr := range vxlanDevAddrList {
		if _, exist := nodeLocalVxlanAddrMap[addr.IP.String()]; !exist {
			if err := netlink.AddrDel(vxlanDev.Link(), &addr); err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("failed to del addr %v for link %v: %v", addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	for _, node := range nodeList.Items {
		if node.Annotations[constants.AnnotationNodeVtepMac] == "" ||
			node.Annotations[constants.AnnotationNodeVtepIP] == "" ||
			node.Annotations[constants.AnnotationNodeLocalVxlanIPList] == "" {
			logger.Info("node's vtep information has not been updated", "node", node.Name)
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

	return reconcile.Result{}, nil
}

func checkNodeUpdate(updateEvent event.UpdateEvent) bool {
	oldNode := updateEvent.ObjectOld.(*corev1.Node)
	newNode := updateEvent.ObjectNew.(*corev1.Node)

	if oldNode.Annotations[constants.AnnotationNodeVtepIP] != newNode.Annotations[constants.AnnotationNodeVtepIP] ||
		oldNode.Annotations[constants.AnnotationNodeVtepMac] != newNode.Annotations[constants.AnnotationNodeVtepMac] {
		return true
	}
	return false
}
