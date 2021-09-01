package route

import (
	"fmt"
	"net"

	daemonutils "github.com/oecp/rama/pkg/daemon/utils"
)

func (m *Manager) ResetRemoteInfos() {
	m.remoteOverlaySubnetInfoMap = SubnetInfoMap{}
	m.remoteUnderlaySubnetInfoMap = SubnetInfoMap{}
	m.remoteSubnetTracker.Refresh()
	m.remoteCidr.Clear()
}

func (m *Manager) AddRemoteSubnetInfo(cluster string, cidr *net.IPNet, gateway, start, end net.IP, excludeIPs []net.IP, isOverlay bool) error {
	cidrString := cidr.String()

	if err := m.remoteSubnetTracker.Track(cidrString, cluster); err != nil {
		return err
	}

	var subnetInfo *SubnetInfo
	if isOverlay {
		if _, exists := m.remoteOverlaySubnetInfoMap[cidrString]; !exists {
			m.remoteOverlaySubnetInfoMap[cidrString] = &SubnetInfo{
				cidr:             cidr,
				gateway:          gateway,
				includedIPRanges: []*daemonutils.IPRange{},
				excludeIPs:       []net.IP{},
			}
		}

		subnetInfo = m.remoteOverlaySubnetInfoMap[cidrString]
	} else {
		if _, exists := m.remoteUnderlaySubnetInfoMap[cidrString]; !exists {
			m.remoteUnderlaySubnetInfoMap[cidrString] = &SubnetInfo{
				cidr:             cidr,
				gateway:          gateway,
				includedIPRanges: []*daemonutils.IPRange{},
				excludeIPs:       []net.IP{},
			}
		}

		subnetInfo = m.remoteUnderlaySubnetInfoMap[cidrString]
	}

	if len(excludeIPs) != 0 {
		subnetInfo.excludeIPs = append(subnetInfo.excludeIPs, excludeIPs...)
	}

	if start != nil || end != nil {
		if start == nil {
			start = cidr.IP
		}

		if end == nil {
			end = daemonutils.LastIP(cidr)
		}

		if ipRange, _ := daemonutils.CreateIPRange(start, end); ipRange != nil {
			subnetInfo.includedIPRanges = append(subnetInfo.includedIPRanges, ipRange)
		}
	}

	m.remoteCidr.Add(cidrString)
	return nil
}

func (m *Manager) configureRemote() (bool, error) {
	if len(m.remoteOverlaySubnetInfoMap) == 0 && len(m.remoteUnderlaySubnetInfoMap) == 0 {
		return false, nil
	}

	if err := m.remoteSubnetTracker.Conflict(); err != nil {
		return false, err
	}

	if conflict := m.remoteCidr.Intersect(m.localCidr); conflict.Size() > 0 {
		return false, fmt.Errorf("local cluster and remote clusters have a conflict in subnet config [%s]", conflict)
	}

	return true, nil
}
