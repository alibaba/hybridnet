package iptables

import (
	"fmt"
	"net"
)

func (mgr *Manager) ResetRemote() {
	mgr.remoteOverlaySubnet = []*net.IPNet{}
	mgr.remoteUnderlaySubnet = []*net.IPNet{}
	mgr.remoteNodeIPList = []net.IP{}
	mgr.remoteSubnetTracker.Refresh()
	mgr.remoteCidr.Clear()
}

func (mgr *Manager) RecordRemoteNodeIP(nodeIP net.IP) {
	mgr.remoteNodeIPList = append(mgr.remoteNodeIPList, nodeIP)
}

func (mgr *Manager) RecordRemoteSubnet(cluster string, subnetCidr *net.IPNet, isOverlay bool) error {
	if err := mgr.remoteSubnetTracker.Track(subnetCidr.String(), cluster); err != nil {
		return err
	}

	if isOverlay {
		mgr.remoteOverlaySubnet = append(mgr.remoteOverlaySubnet, subnetCidr)
	} else {
		mgr.remoteUnderlaySubnet = append(mgr.remoteUnderlaySubnet, subnetCidr)
	}

	mgr.remoteCidr.Add(subnetCidr.String())
	return nil
}

func (mgr *Manager) configureRemote() (bool, error) {
	if len(mgr.remoteOverlaySubnet) == 0 && len(mgr.remoteUnderlaySubnet) == 0 {
		return false, nil
	}

	if err := mgr.remoteSubnetTracker.Conflict(); err != nil {
		return false, err
	}

	if conflict := mgr.remoteCidr.Intersect(mgr.localCidr); conflict.Size() > 0 {
		return false, fmt.Errorf("local cluster and remote clusters have a conflict in subnet config [%s]", conflict)
	}

	return true, nil
}
