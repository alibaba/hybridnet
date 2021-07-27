package controller

import (
	"fmt"
	"net"
	"reflect"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

type RemoteVtepCache struct {
	remoteVtepIPMap map[string]net.HardwareAddr

	ch chan struct{}
}

func NewRemoteVtepCache() *RemoteVtepCache {
	return &RemoteVtepCache{
		remoteVtepIPMap: map[string]net.HardwareAddr{},
		ch:              make(chan struct{}, 1),
	}
}

func (rvc *RemoteVtepCache) UpdateRemoteVtepIPs(remoteVteps []*ramav1.RemoteVtep) error {
	rvc.lock()
	defer rvc.unlock()

	rvc.remoteVtepIPMap = map[string]net.HardwareAddr{}

	for _, remoteVtep := range remoteVteps {
		macAddr, err := net.ParseMAC(remoteVtep.Spec.VtepMAC)
		if err != nil {
			return fmt.Errorf("parse node vtep mac %v failed: %v", remoteVtep.Spec.VtepMAC, err)
		}

		rvc.remoteVtepIPMap[remoteVtep.Spec.VtepIP] = macAddr
	}

	return nil
}

func (rvc *RemoteVtepCache) SearchIP(ip net.IP) (net.HardwareAddr, bool) {
	rvc.lock()
	defer rvc.unlock()

	mac, exist := rvc.remoteVtepIPMap[ip.String()]
	return mac, exist
}

func (rvc *RemoteVtepCache) lock() {
	rvc.ch <- struct{}{}
}

func (rvc *RemoteVtepCache) unlock() {
	<-rvc.ch
}

// add handler for RemoteVtep and RemoteSubnet

func (c *Controller) enqueueAddOrDeleteRemoteVtep(obj interface{}) {
	c.nodeQueue.Add(ActionReconcileNode)
}

func (c *Controller) enqueueUpdateRemoteVtep(oldObj, newObj interface{}) {
	oldRv := oldObj.(*ramav1.RemoteVtep)
	newRv := newObj.(*ramav1.RemoteVtep)

	if oldRv.Spec.VtepIP != newRv.Spec.VtepIP ||
		oldRv.Spec.VtepMAC != newRv.Spec.VtepMAC || !reflect.DeepEqual(oldRv.Status.PodIPList, newRv.Status.PodIPList) {
		c.nodeQueue.Add(ActionReconcileNode)
	}
}

func (c *Controller) enqueueAddOrDeleteRemoteSubnet(obj interface{}) {
	c.remoteSubnetQueue.Add(ActionReconcileRemoteSubnet)
}

func (c *Controller) enqueueUpdateRemoteSubnet(oldObj, newObj interface{}) {
	oldRs := oldObj.(*ramav1.RemoteSubnet)
	newRs := newObj.(*ramav1.RemoteSubnet)

	oldRsNetID := oldRs.Spec.OverlayNetID
	newRsNetID := newRs.Spec.OverlayNetID

	if (oldRsNetID == nil && newRsNetID != nil) ||
		(oldRsNetID != nil && newRsNetID == nil) ||
		(oldRsNetID != nil && newRsNetID != nil && *oldRsNetID != *newRsNetID) ||
		oldRs.Spec.CIDR != newRs.Spec.CIDR ||
		ramav1.GetRemoteSubnetType(oldRs) != ramav1.GetRemoteSubnetType(newRs) {
		c.remoteSubnetQueue.Add(ActionReconcileRemoteSubnet)
	}
}

func (c *Controller) processNextRemoteSubnetWorkItem() bool {
	obj, shutdown := c.remoteSubnetQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.remoteSubnetQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.remoteSubnetQueue.Forget(obj)
			klog.Errorf("expected string in work queue but got %#v", obj)
			return nil
		}
		if err := c.reconcileRemoteSubnet(); err != nil {
			c.remoteSubnetQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.remoteSubnetQueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}
	return true
}

func (c *Controller) runRemoteSubnetWorker() {
	for c.processNextRemoteSubnetWorkItem() {
	}
}

func (c *Controller) reconcileRemoteSubnet() error {
	klog.Info("Reconciling remote subnet information")

	remoteSubnetList, err := c.remoteSubnetLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list remote subnet %v", err)
	}

	c.routeV4Manager.ResetRemoteInfos()
	c.routeV6Manager.ResetRemoteInfos()

	for _, remoteSubnet := range remoteSubnetList {
		subnetCidr, idErr := netlink.ParseIPNet(remoteSubnet.Spec.CIDR)
		if idErr != nil {
			return fmt.Errorf("failed to parse remote subnet cidr %v error: %v", remoteSubnet.Spec.CIDR, err)
		}

		switch ramav1.GetRemoteSubnetType(remoteSubnet) {
		case ramav1.NetworkTypeUnderlay:
			c.getRouterManager(remoteSubnet.Spec.Version).AddRemoteUnderlaySubnetInfo(subnetCidr)
		case ramav1.NetworkTypeOverlay:
			var netID = remoteSubnet.Spec.OverlayNetID
			if netID == nil {
				return fmt.Errorf("a remote overlay subnet [%v] misses its net id", remoteSubnet.Name)
			}
			forwardNodeIfName, vxErr := containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, netID)
			if vxErr != nil {
				return fmt.Errorf("generate vxlan forward node if name failed: %v", err)
			}
			c.getRouterManager(remoteSubnet.Spec.Version).AddRemoteOverlaySubnetInfo(subnetCidr, forwardNodeIfName)
		}
	}

	if err = c.routeV4Manager.SyncRoutes(); err != nil {
		return fmt.Errorf("sync ipv4 routes failed: %v", err)
	}

	if err = c.routeV6Manager.SyncRoutes(); err != nil {
		return fmt.Errorf("sync ipv6 routes failed: %v", err)
	}

	c.iptablesSyncTrigger()

	return nil
}
