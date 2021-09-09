package iptables

import (
	"net"
)

func (mgr *Manager) RecordRemoteNodeIP(nodeIP net.IP) {
	mgr.remoteNodeIPList = append(mgr.remoteNodeIPList, nodeIP)
}

func (mgr *Manager) RecordRemoteSubnet(cluster string, subnetCidr *net.IPNet, isOverlay bool) error {
	if isOverlay {
		mgr.remoteOverlaySubnet = append(mgr.remoteOverlaySubnet, subnetCidr)
	} else {
		mgr.remoteUnderlaySubnet = append(mgr.remoteUnderlaySubnet, subnetCidr)
	}
	return nil
}
