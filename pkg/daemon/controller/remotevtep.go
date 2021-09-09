package controller

import (
	"fmt"
	"net"
	"sync"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
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
