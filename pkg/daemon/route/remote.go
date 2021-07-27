package route

import (
	"fmt"
	"net"
)

const (
	RemoteOverlayNotExists = "#nonexist"
	RemoteOverlayConflict  = "#conflict"
)

func (m *Manager) ResetRemoteInfos() {
	m.remoteOverlaySubnets = map[string]*net.IPNet{}
	m.remoteUnderlaySubnets = map[string]*net.IPNet{}
	m.remoteOverlayIfName = RemoteOverlayNotExists
}

func (m *Manager) AddRemoteOverlaySubnetInfo(cidr *net.IPNet, forwardNodeIfName string) {
	m.remoteOverlaySubnets[cidr.String()] = cidr

	// fresh remoteOverlayIfName
	switch m.remoteOverlayIfName {
	case RemoteOverlayNotExists:
		m.remoteOverlayIfName = forwardNodeIfName
	case RemoteOverlayConflict:
	default:
		if m.remoteOverlayIfName != forwardNodeIfName {
			m.remoteOverlayIfName = RemoteOverlayConflict
		}
	}
}

func (m *Manager) AddRemoteUnderlaySubnetInfo(cidr *net.IPNet) {
	m.remoteUnderlaySubnets[cidr.String()] = cidr
}

func (m *Manager) GetRemoteSubnets() (Overlay, Underlay map[string]*net.IPNet) {
	return m.remoteOverlaySubnets, m.remoteUnderlaySubnets
}

func (m *Manager) configureRemote() (bool, error) {
	if len(m.remoteOverlaySubnets) == 0 && len(m.remoteUnderlaySubnets) == 0 {
		return false, nil
	}

	if len(m.remoteOverlaySubnets) != 0 {
		if m.remoteOverlayIfName != RemoteOverlayConflict &&
			m.remoteOverlayIfName != RemoteOverlayNotExists &&
			(m.overlayIfName == "" || m.remoteOverlayIfName == m.overlayIfName) {
			return true, nil
		}

		return false, fmt.Errorf("invalid remote overlay net interface configuration [local=%s, remote=%s]", m.overlayIfName, m.remoteOverlayIfName)
	}

	return true, nil
}

func (m *Manager) isValidRemoteOverlayIfName() bool {
	return m.remoteOverlayIfName != RemoteOverlayNotExists && m.remoteOverlayIfName != RemoteOverlayConflict
}
