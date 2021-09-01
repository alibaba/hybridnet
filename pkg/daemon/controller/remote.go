package controller

import (
	"fmt"
	"net"
	"sync"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

type RemoteVtepCache struct {
	remoteVtepIPMap map[string]net.HardwareAddr
	mu              *sync.RWMutex
}

func NewRemoteVtepCache() *RemoteVtepCache {
	return &RemoteVtepCache{
		remoteVtepIPMap: map[string]net.HardwareAddr{},
		mu:              &sync.RWMutex{},
	}
}

func (rvc *RemoteVtepCache) UpdateRemoteVtepIPs(remoteVteps []*ramav1.RemoteVtep) error {
	rvc.mu.Lock()
	defer rvc.mu.Unlock()

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
	rvc.mu.RLock()
	defer rvc.mu.RUnlock()

	mac, exist := rvc.remoteVtepIPMap[ip.String()]
	return mac, exist
}

// add handler for RemoteVtep and RemoteSubnet

func (c *Controller) enqueueAddOrDeleteRemoteVtep(obj interface{}) {
	c.nodeQueue.Add(ActionReconcileNode)
}

func (c *Controller) enqueueUpdateRemoteVtep(oldObj, newObj interface{}) {
	oldRv := oldObj.(*ramav1.RemoteVtep)
	newRv := newObj.(*ramav1.RemoteVtep)

	if oldRv.Spec.VtepIP != newRv.Spec.VtepIP ||
		oldRv.Spec.VtepMAC != newRv.Spec.VtepMAC ||
		!isIPListEqual(oldRv.Spec.EndpointIPList, newRv.Spec.EndpointIPList) {
		c.nodeQueue.Add(ActionReconcileNode)
	}
}

func (c *Controller) enqueueAddOrDeleteRemoteSubnet(obj interface{}) {
	c.remoteSubnetQueue.Add(ActionReconcileRemoteSubnet)
}

func (c *Controller) enqueueUpdateRemoteSubnet(oldObj, newObj interface{}) {
	oldRs := oldObj.(*ramav1.RemoteSubnet)
	newRs := newObj.(*ramav1.RemoteSubnet)

	if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
		!isAddressRangeEqual(&oldRs.Spec.Range, &newRs.Spec.Range) ||
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
		subnetCidr, gatewayIP, startIP, endIP, excludeIPs,
			_, err := parseSubnetSpecRangeMeta(&remoteSubnet.Spec.Range)

		if err != nil {
			return fmt.Errorf("parse subnet %v spec range meta failed: %v", remoteSubnet.Name, err)
		}

		var isOverlay = ramav1.GetRemoteSubnetType(remoteSubnet) == ramav1.NetworkTypeOverlay

		routeManager := c.getRouterManager(remoteSubnet.Spec.Range.Version)
		err = routeManager.AddRemoteSubnetInfo(remoteSubnet.Spec.ClusterName, subnetCidr, gatewayIP, startIP, endIP, excludeIPs, isOverlay)

		if err != nil {
			return fmt.Errorf("failed to add remote subnet info: %v", err)
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
