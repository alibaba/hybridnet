package iptables

import (
	"fmt"
	"net"
)

const (
	RemoteOverlayNotExists = "#nonexist"
	RemoteOverlayConflict  = "#conflict"
)

func (mgr *Manager) ResetRemote() {
	mgr.remoteOverlaySubnet = []*net.IPNet{}
	mgr.remoteUnderlaySubnet = []*net.IPNet{}
	mgr.remoteNodeIPList = []net.IP{}
	mgr.remoteOverlayIfName = RemoteOverlayNotExists
}

func (mgr *Manager) RecordRemoteNodeIP(nodeIP net.IP) {
	mgr.remoteNodeIPList = append(mgr.remoteNodeIPList, nodeIP)
}

func (mgr *Manager) RecordRemoteSubnet(subnetCidr *net.IPNet, isOverlay bool) {
	if isOverlay {
		mgr.remoteOverlaySubnet = append(mgr.remoteOverlaySubnet, subnetCidr)
	} else {
		mgr.remoteUnderlaySubnet = append(mgr.remoteUnderlaySubnet, subnetCidr)
	}
}

func (mgr *Manager) SetRemoteOverlayIfName(overlayIfName string) {
	switch mgr.remoteOverlayIfName {
	case RemoteOverlayNotExists:
		mgr.remoteOverlayIfName = overlayIfName
	case RemoteOverlayConflict:
	default:
		if mgr.remoteOverlayIfName != overlayIfName {
			mgr.remoteOverlayIfName = RemoteOverlayConflict
		}
	}
}

func (mgr *Manager) configureRemote() (bool, error) {
	if len(mgr.remoteOverlaySubnet) == 0 && len(mgr.remoteUnderlaySubnet) == 0 {
		return false, nil
	}

	if len(mgr.remoteOverlaySubnet) != 0 {
		if mgr.remoteOverlayIfName != RemoteOverlayConflict &&
			mgr.remoteOverlayIfName != RemoteOverlayNotExists &&
			(mgr.overlayIfName == "" || mgr.remoteOverlayIfName == mgr.overlayIfName) {
			return true, nil
		}

		return false, fmt.Errorf("invalid remote overlay net interface configuration [local=%s, remote=%s]", mgr.overlayIfName, mgr.remoteOverlayIfName)
	}

	return true, nil
}

func (mgr *Manager) isValidRemoteOverlayIfName() bool {
	return mgr.remoteOverlayIfName != RemoteOverlayNotExists && mgr.remoteOverlayIfName != RemoteOverlayConflict
}
