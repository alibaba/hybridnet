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
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	"github.com/oecp/rama/pkg/daemon/vxlan"

	"golang.org/x/sys/unix"

	"github.com/vishvananda/netlink"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

type NodeIPCache struct {
	// ip string to node vtep mac
	nodeIPMap map[string]net.HardwareAddr
	mu        *sync.RWMutex
}

func NewNodeIPCache() *NodeIPCache {
	return &NodeIPCache{
		nodeIPMap: map[string]net.HardwareAddr{},
		mu:        &sync.RWMutex{},
	}
}

func (nic *NodeIPCache) UpdateNodeIPs(nodeList []*v1.Node, localNodeName string) error {
	nic.mu.Lock()
	defer nic.mu.Unlock()

	nic.nodeIPMap = map[string]net.HardwareAddr{}
	for _, node := range nodeList {
		// Only update remote node vtep information.
		if node.Name == localNodeName {
			continue
		}

		// ignore empty information
		if _, exist := node.Annotations[constants.AnnotationNodeVtepMac]; !exist {
			continue
		}

		macAddr, err := net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
		if err != nil {
			return fmt.Errorf("parse node vtep mac %v failed: %v", node.Annotations[constants.AnnotationNodeVtepMac], err)
		}

		ipStringList := strings.Split(node.Annotations[constants.AnnotationNodeLocalVxlanIPList], ",")
		for _, ipString := range ipStringList {
			nic.nodeIPMap[ipString] = macAddr
		}
	}

	return nil
}

func (nic *NodeIPCache) SearchIP(ip net.IP) (net.HardwareAddr, bool) {
	nic.mu.RLock()
	defer nic.mu.RUnlock()

	mac, exist := nic.nodeIPMap[ip.String()]
	return mac, exist
}

// reconcile node infos on node if node info changed
func (c *Controller) enqueueAddOrDeleteNode(obj interface{}) {
	c.nodeQueue.Add(ActionReconcileNode)
}

func (c *Controller) enqueueUpdateNode(oldObj, newObj interface{}) {
	oldNode := oldObj.(*v1.Node)
	newNode := newObj.(*v1.Node)

	if oldNode.Annotations[constants.AnnotationNodeVtepIP] != newNode.Annotations[constants.AnnotationNodeVtepIP] ||
		oldNode.Annotations[constants.AnnotationNodeVtepMac] != newNode.Annotations[constants.AnnotationNodeVtepMac] {

		c.nodeQueue.Add(ActionReconcileNode)

		// For iptables rule update.
		c.subnetQueue.Add(ActionReconcileSubnet)
		return
	}
}

func (c *Controller) processNextNodeWorkItem() bool {
	obj, shutdown := c.nodeQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.nodeQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.nodeQueue.Forget(obj)
			klog.Errorf("expected string in work queue but got %#v", obj)
			return nil
		}
		if err := c.reconcileNodeInfo(); err != nil {
			c.nodeQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.nodeQueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}
	return true
}

func (c *Controller) runNodeWorker() {
	for c.processNextNodeWorkItem() {
	}
}

func (c *Controller) reconcileNodeInfo() error {
	klog.Info("Reconciling node information")

	networkList, err := c.networkLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list network %v", err)
	}

	if err != nil {
		return fmt.Errorf("failed to list remote subnet %v", err)
	}

	var overlayNetID *uint32
	for _, network := range networkList {
		if ramav1.GetNetworkType(network) == ramav1.NetworkTypeOverlay {
			overlayNetID = network.Spec.NetID
			break
		}
	}

	// overlay network not exist, do nothing
	if overlayNetID == nil {
		return nil
	}

	link, err := netlink.LinkByName(c.config.NodeVxlanIfName)
	if err != nil {
		return fmt.Errorf("get node vxlan interface %v failed: %v", c.config.NodeVxlanIfName, err)
	}

	// Use parent's valid ipv4 address first, try ipv6 address if no valid ipv4 address exist.
	existParentAddrList, err := containernetwork.ListAllAddress(link)
	if err != nil {
		return fmt.Errorf("list address for vxlan parent link %v failed %v", link.Attrs().Name, err)
	}

	if len(existParentAddrList) == 0 {
		return fmt.Errorf("there is no available ip for vxlan parent link %v", link.Attrs().Name)
	}

	var vtepIP net.IP
	for _, addr := range existParentAddrList {
		if addr.IP.IsGlobalUnicast() {
			vtepIP = addr.IP
			break
		}
	}

	thisNode, err := c.nodeLister.Get(c.config.NodeName)
	if err != nil {
		return fmt.Errorf("get this node %v object failed: %v", c.config.NodeName, err)
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
		return fmt.Errorf("no availuable vtep ip can be used for link %v", link.Attrs().Name)
	}
	vtepMac := link.Attrs().HardwareAddr

	vxlanLinkName, err := containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, overlayNetID)
	if err != nil {
		return fmt.Errorf("generate vxlan interface name failed: %v", err)
	}

	existAllAddrList, err := containernetwork.ListLocalAddressExceptLink(vxlanLinkName)
	if err != nil {
		return fmt.Errorf("list address for all interfaces failed: %v", err)
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
		for _, cidr := range c.config.ExtraNodeLocalVxlanIPCidrs {
			if cidr.Contains(addr.IP) {
				nodeLocalVxlanAddr = append(nodeLocalVxlanAddr, addr)
			}
		}
	}

	patchData := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s","%s":"%s","%s":"%s"}}}`,
		constants.AnnotationNodeVtepIP, vtepIP.String(),
		constants.AnnotationNodeVtepMac, vtepMac.String(),
		constants.AnnotationNodeLocalVxlanIPList, containernetwork.GenerateIPListString(nodeLocalVxlanAddr))

	if _, err := c.config.KubeClient.CoreV1().Nodes().Patch(context.TODO(),
		c.config.NodeName, types.StrategicMergePatchType,
		[]byte(patchData), metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("update node label error: %v", err)
	}

	nodeList, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list node failed: %v", err)
	}

	remoteVtepList, err := c.remoteVtepLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list remote vtep failed: %v", err)
	}

	vxlanDev, err := vxlan.NewVxlanDevice(vxlanLinkName, int(*overlayNetID),
		c.config.NodeVxlanIfName, vtepIP, c.config.VxlanUDPPort, c.config.VxlanBaseReachableTime, true)
	if err != nil {
		return fmt.Errorf("create vxlan device %v failed: %v", vxlanLinkName, err)
	}

	nodeLocalVxlanAddrMap := map[string]bool{}
	for _, addr := range nodeLocalVxlanAddr {
		nodeLocalVxlanAddrMap[addr.IP.String()] = true
	}

	vxlanDevAddrList, err := containernetwork.ListAllAddress(vxlanDev.Link())
	if err != nil {
		return fmt.Errorf("list address for vxlan interface %v failed: %v", vxlanDev.Link().Name, err)
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
				return fmt.Errorf("set addr %v to link %v failed: %v", addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	// Delete invalid address.
	for _, addr := range vxlanDevAddrList {
		if _, exist := nodeLocalVxlanAddrMap[addr.IP.String()]; !exist {
			if err := netlink.AddrDel(vxlanDev.Link(), &addr); err != nil {
				return fmt.Errorf("del addr %v for link %v failed: %v", addr.IP.String(), vxlanDev.Link().Name, err)
			}
		}
	}

	if err := c.nodeIPCache.UpdateNodeIPs(nodeList, c.config.NodeName); err != nil {
		return fmt.Errorf("update node ip cache failed: %v", err)
	}

	if err := c.remoteVtepCache.UpdateRemoteVtepIPs(remoteVtepList); err != nil {
		return fmt.Errorf("update remote vtep ip cache failed: %v", err)
	}

	for _, node := range nodeList {
		if node.Annotations[constants.AnnotationNodeVtepMac] == "" ||
			node.Annotations[constants.AnnotationNodeVtepIP] == "" ||
			node.Annotations[constants.AnnotationNodeLocalVxlanIPList] == "" {
			klog.Infof("node %v's vtep information has not been updated", node.Name)
			continue
		}

		vtepMac, err := net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
		if err != nil {
			return fmt.Errorf("parse node vtep mac string %v failed: %v", node.Annotations[constants.AnnotationNodeVtepMac], err)
		}

		vtepIP := net.ParseIP(node.Annotations[constants.AnnotationNodeVtepIP])
		if vtepIP == nil {
			return fmt.Errorf("parse node vtep ip string %v failed", node.Annotations[constants.AnnotationNodeVtepIP])
		}

		vxlanDev.RecordVtepInfo(vtepMac, vtepIP)
	}

	for _, remoteVtep := range remoteVtepList {
		vtepMac, err := net.ParseMAC(remoteVtep.Spec.VtepMAC)
		if err != nil {
			return fmt.Errorf("parse node vtep mac string %v failed: %v", remoteVtep.Spec.VtepMAC, err)
		}

		vtepIP := net.ParseIP(remoteVtep.Spec.VtepIP)
		if vtepIP == nil {
			return fmt.Errorf("parse node vtep ip string %v failed", remoteVtep.Spec.VtepIP)
		}

		vxlanDev.RecordRemoteVtepInfo(vtepMac, vtepIP)
	}

	if err := vxlanDev.SyncVtepInfo(); err != nil {
		return fmt.Errorf("sync vtep info for vxlan device %v failed: %v", vxlanDev.Link().Name, err)
	}

	c.iptablesSyncTrigger()

	return nil
}
