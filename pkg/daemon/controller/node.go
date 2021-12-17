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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
	"github.com/alibaba/hybridnet/pkg/daemon/vxlan"
	"github.com/alibaba/hybridnet/pkg/feature"

	"golang.org/x/sys/unix"

	"github.com/vishvananda/netlink"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

type nodeReconciler struct {
	client.Client
	controllerRef *Controller
}

func (r *nodeReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Info("Reconciling node information")

	networkList := &networkingv1.NetworkList{}
	if err := r.List(ctx, networkList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list network %v", err)
	}

	var overlayNetID *uint32
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

	link, err := netlink.LinkByName(r.controllerRef.config.NodeVxlanIfName)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("get node vxlan interface %v failed: %v",
			r.controllerRef.config.NodeVxlanIfName, err)
	}

	// Use parent's valid ipv4 address first, try ipv6 address if no valid ipv4 address exist.
	existParentAddrList, err := containernetwork.ListAllAddress(link)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("list address for vxlan parent link %v failed %v",
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
	if err := r.Get(ctx, types.NamespacedName{Name: r.controllerRef.config.NodeName}, thisNode); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("get this node %v object failed: %v",
			r.controllerRef.config.NodeName, err)
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

	vxlanLinkName, err := containernetwork.GenerateVxlanNetIfName(r.controllerRef.config.NodeVxlanIfName, overlayNetID)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("generate vxlan interface name failed: %v", err)
	}

	existAllAddrList, err := containernetwork.ListLocalAddressExceptLink(vxlanLinkName)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("list address for all interfaces failed: %v", err)
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
		for _, cidr := range r.controllerRef.config.ExtraNodeLocalVxlanIPCidrs {
			if cidr.Contains(addr.IP) {
				nodeLocalVxlanAddr = append(nodeLocalVxlanAddr, addr)
			}
		}
	}

	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s","%s":"%s","%s":"%s"}}}`,
		constants.AnnotationNodeVtepIP, vtepIP.String(),
		constants.AnnotationNodeVtepMac, vtepMac.String(),
		constants.AnnotationNodeLocalVxlanIPList, containernetwork.GenerateIPListString(nodeLocalVxlanAddr))

	if err := r.controllerRef.mgr.GetClient().Patch(context.TODO(),
		thisNode, client.RawPatch(types.StrategicMergePatchType, []byte(patchData))); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("update node label error: %v", err)
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("list node failed: %v", err)
	}

	vxlanDev, err := vxlan.NewVxlanDevice(vxlanLinkName, int(*overlayNetID),
		r.controllerRef.config.NodeVxlanIfName, vtepIP, r.controllerRef.config.VxlanUDPPort,
		r.controllerRef.config.VxlanBaseReachableTime, true)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("create vxlan device %v failed: %v", vxlanLinkName, err)
	}

	nodeLocalVxlanAddrMap := map[string]bool{}
	for _, addr := range nodeLocalVxlanAddr {
		nodeLocalVxlanAddrMap[addr.IP.String()] = true
	}

	vxlanDevAddrList, err := containernetwork.ListAllAddress(vxlanDev.Link())
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("list address for vxlan interface %v failed: %v",
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
				return reconcile.Result{Requeue: true}, fmt.Errorf("set addr %v to link %v failed: %v",
					addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	// Delete invalid address.
	for _, addr := range vxlanDevAddrList {
		if _, exist := nodeLocalVxlanAddrMap[addr.IP.String()]; !exist {
			if err := netlink.AddrDel(vxlanDev.Link(), &addr); err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("del addr %v for link %v failed: %v", addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	for _, node := range nodeList.Items {
		if node.Annotations[constants.AnnotationNodeVtepMac] == "" ||
			node.Annotations[constants.AnnotationNodeVtepIP] == "" ||
			node.Annotations[constants.AnnotationNodeLocalVxlanIPList] == "" {
			klog.Infof("node %v's vtep information has not been updated", node.Name)
			continue
		}

		vtepMac, err := net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
		if err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("parse node vtep mac string %v failed: %v",
				node.Annotations[constants.AnnotationNodeVtepMac], err)
		}

		vtepIP := net.ParseIP(node.Annotations[constants.AnnotationNodeVtepIP])
		if vtepIP == nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("parse node vtep ip string %v failed",
				node.Annotations[constants.AnnotationNodeVtepIP])
		}

		vxlanDev.RecordVtepInfo(vtepMac, vtepIP)
	}

	var remoteVtepList []*networkingv1.RemoteVtep

	if feature.MultiClusterEnabled() {
		remoteVtepList := &networkingv1.RemoteVtepList{}
		if err = r.List(ctx, remoteVtepList); err != nil {
			return reconcile.Result{Requeue: true}, fmt.Errorf("list remote vtep failed: %v", err)
		}

		for _, remoteVtep := range remoteVtepList.Items {
			vtepMac, err := net.ParseMAC(remoteVtep.Spec.VtepMAC)
			if err != nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("parse remote vtep mac string %v failed: %v",
					remoteVtep.Spec.VtepMAC, err)
			}

			vtepIP := net.ParseIP(remoteVtep.Spec.VtepIP)
			if vtepIP == nil {
				return reconcile.Result{Requeue: true}, fmt.Errorf("parse remote vtep ip string %v failed",
					remoteVtep.Spec.VtepIP)
			}

			vxlanDev.RecordVtepInfo(vtepMac, vtepIP)
		}
	}

	if err := r.controllerRef.nodeIPCache.UpdateNodeIPs(nodeList.Items, r.controllerRef.config.NodeName, remoteVtepList); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("update node ip cache failed: %v", err)
	}

	if err := vxlanDev.SyncVtepInfo(); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("sync vtep info for vxlan device %v failed: %v",
			vxlanDev.Link().Name, err)
	}

	r.controllerRef.iptablesSyncTrigger()

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
