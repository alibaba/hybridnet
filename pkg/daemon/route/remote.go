package route

import (
	"net"

	daemonutils "github.com/oecp/rama/pkg/daemon/utils"
)

func (m *Manager) AddRemoteSubnetInfo(cidr *net.IPNet, gateway, start, end net.IP, excludeIPs []net.IP, isOverlay bool) error {
	cidrString := cidr.String()

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

	return nil
}
