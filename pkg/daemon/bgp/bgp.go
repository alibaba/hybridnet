/*
 Copyright 2021 The Hybridnet Authors.

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

package bgp

import (
	"context"
	"fmt"
	"net"

	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"

	"github.com/vishvananda/netlink"

	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"

	"github.com/go-logr/logr"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/server"
)

// Multiple bgp peers is not supported right now.

type Manager struct {
	localASN             uint32
	peeringInterfaceName string

	routerID string
	// choose next hop address when advertise ipv4 address
	routerV4Address net.IP
	// choose next hop address when advertise ipv6 address
	routerV6Address net.IP

	bgpServer *server.BgpServer

	logger logr.Logger

	peerMap       map[string]*peerInfo
	subnetMap     map[string]*net.IPNet
	ipInstanceMap map[string]net.IP
}

func NewManager(peeringInterfaceName, grpcListenAddress string, logger logr.Logger) (*Manager, error) {
	manager := &Manager{
		// For using gobgp cmd to debug
		bgpServer: server.NewBgpServer(
			server.GrpcListenAddress(grpcListenAddress),
			server.LoggerOption(&bgpLogger{logger: logger.WithName("gobgpd")}),
		),

		logger:               logger,
		peeringInterfaceName: peeringInterfaceName,

		peerMap:       map[string]*peerInfo{},
		subnetMap:     map[string]*net.IPNet{},
		ipInstanceMap: map[string]net.IP{},
	}

	peeringLink, err := netlink.LinkByName(peeringInterfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bgp peering link %v: %v", peeringInterfaceName, err)
	}

	existLinkAddress, err := containernetwork.ListAllAddress(peeringLink)
	if err != nil {
		return nil, fmt.Errorf("failed to get link address for bgp peering interface %v: %v", peeringInterfaceName, err)
	}

	switch len(existLinkAddress) {
	case 0:
		return nil, fmt.Errorf("there is no valid address on bpg peering interface")
	case 1:
		manager.routerID = existLinkAddress[0].IP.String()
		if existLinkAddress[0].IP.To4() == nil {
			manager.routerV6Address = existLinkAddress[0].IP
		} else {
			manager.routerV4Address = existLinkAddress[0].IP
		}
	case 2:
		if (existLinkAddress[0].IP.To4() != nil && existLinkAddress[1].IP.To4() == nil) ||
			(existLinkAddress[0].IP.To4() == nil && existLinkAddress[1].IP.To4() != nil) {
			for _, addr := range existLinkAddress {
				if addr.IP.To4() == nil {
					manager.routerV6Address = addr.IP
				}
				manager.routerV4Address = addr.IP
			}

			// Use v4 address as routerID by default if v4/v6 addresses exist at the same time.
			manager.routerID = manager.routerV4Address.String()
			break
		}
		fallthrough
	default:
		defaultV4Route, err := containernetwork.GetDefaultRoute(netlink.FAMILY_V4)
		if err != nil && err != daemonutils.NotExist {
			return nil, fmt.Errorf("failed to get v4 default route: %v", err)
		}

		defaultV6Route, err := containernetwork.GetDefaultRoute(netlink.FAMILY_V6)
		if err != nil && err != daemonutils.NotExist {
			return nil, fmt.Errorf("failed to get v6 default route: %v", err)
		}

		for _, addr := range existLinkAddress {
			if defaultV4Route != nil {
				if addr.IP.Equal(defaultV4Route.Src) || addr.IPNet.Contains(defaultV4Route.Gw) {
					manager.routerV4Address = addr.IP
				}
			}

			if defaultV6Route != nil {
				if addr.IP.Equal(defaultV6Route.Src) || addr.IPNet.Contains(defaultV6Route.Gw) {
					manager.routerV6Address = addr.IP
				}
			}
		}

		if manager.routerV4Address == nil && manager.routerV6Address == nil {
			return nil, fmt.Errorf("failed to find valid address for bgp router")
		}

		if manager.routerV4Address != nil {
			// Use v4 address as routerID by default if v4/v6 addresses exist at the same time.
			manager.routerID = manager.routerV4Address.String()
		} else {
			manager.routerID = manager.routerV6Address.String()
		}
	}

	go manager.bgpServer.Serve()
	return manager, nil
}

func (m *Manager) RecordPeer(address, password string, asn int, gracefulRestartTime int32) {
	if gracefulRestartTime == 0 {
		gracefulRestartTime = 300
	}

	m.peerMap[address] = &peerInfo{
		address:                address,
		asn:                    asn,
		gracefulRestartSeconds: uint32(gracefulRestartTime),
		password:               password,
	}
}

func (m *Manager) RecordSubnet(cidr *net.IPNet) {
	m.subnetMap[cidr.String()] = cidr
}

func (m *Manager) RecordIPInstance(ip net.IP) {
	m.ipInstanceMap[ip.String()] = ip
}

func (m *Manager) ResetInfos() {
	m.peerMap = map[string]*peerInfo{}
	m.subnetMap = map[string]*net.IPNet{}
	m.ipInstanceMap = map[string]net.IP{}
}

func (m *Manager) TryStart(asn uint32) error {
	if m.localASN == 0 {
		m.localASN = asn
	} else if m.localASN != asn {
		return fmt.Errorf("can not restart bgp manager (local AS number: %v) with a different AS number %v",
			m.localASN, asn)
	} else {
		return nil
	}

	return m.bgpServer.StartBgp(context.Background(), &api.StartBgpRequest{
		Global: &api.Global{
			Asn:      m.localASN,
			RouterId: m.routerID,
		},
	})
}

func (m *Manager) SyncPathsAndPeers() error {
	// Sync peers configuration.
	// Because now UpdatePeer will reset bgp session causing a network fluctuation, we will never update an exist bgp peer.
	existPeerMap := map[string]struct{}{}
	if err := m.bgpServer.ListPeer(context.Background(), &api.ListPeerRequest{EnableAdvertised: true},
		func(peer *api.Peer) {
			existPeerMap[peer.Conf.NeighborAddress] = struct{}{}
		}); err != nil {
		return fmt.Errorf("failed to list bgp peers: %v", err)
	}

	// Don't do any thing if local AS number has not been set.
	if m.localASN == 0 {
		return nil
	}

	for _, peer := range m.peerMap {
		if _, exist := existPeerMap[peer.address]; !exist {
			if err := m.bgpServer.AddPeer(context.Background(), &api.AddPeerRequest{
				Peer: generatePeerConfig(peer),
			}); err != nil {
				return fmt.Errorf("failed to add bgp peer %v: %v", peer.address, err)
			}
		}
	}

	for addr, _ := range existPeerMap {
		if _, exist := m.peerMap[addr]; !exist {
			if err := m.bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{
				Address: addr,
			}); err != nil {
				return fmt.Errorf("failed to add bgp peer %v: %v", addr, err)
			}
		}
	}

	// Sync subnet paths.
	existSubnetPathMap := map[string]*net.IPNet{}
	existIPPathMap := map[string]net.IP{}

	listPathFunc := func(p *api.Destination) {
		// only collect the path generated from local
		if p.Paths[0].NeighborIp == "<nil>" {
			ipAddr, cidr, err := net.ParseCIDR(p.Prefix)
			if err != nil {
				m.logger.Error(err, "failed to parse path prefix", "path-prefix", p.Prefix)
				return
			}

			ones, bits := cidr.Mask.Size()
			// What if the subnet is a /32 or /128 cidr? But maybe it will never happen.
			if ones == bits {
				// this path is generated from ip
				existIPPathMap[ipAddr.String()] = ipAddr
			} else {
				// this path is generated from subnet
				existSubnetPathMap[p.Prefix] = &net.IPNet{
					IP:   ipAddr,
					Mask: cidr.Mask,
				}
			}
		}
	}

	if err := m.bgpServer.ListPath(context.Background(),
		&api.ListPathRequest{Family: v4Family}, listPathFunc); err != nil {
		return fmt.Errorf("failed to list ipv4 path: %v", err)
	}

	if err := m.bgpServer.ListPath(context.Background(),
		&api.ListPathRequest{Family: v6Family}, listPathFunc); err != nil {
		return fmt.Errorf("failed to list ipv6 path: %v", err)
	}

	// Ensure paths for subnets
	for _, subnet := range m.subnetMap {
		nextHop, err := m.getNextHopAddressByIP(subnet.IP)
		if err != nil {
			m.logger.Error(err, "failed to get next hop address to add path for subnet, it will be ignore",
				"subnet", subnet.String())
			continue
		}

		if _, exist := existSubnetPathMap[subnet.String()]; !exist {
			if _, err := m.bgpServer.AddPath(context.Background(), &api.AddPathRequest{
				Path: generatePathForSubnet(subnet, nextHop),
			}); err != nil {
				return fmt.Errorf("failed to add path for subnet %v: %v", subnet.String(), err)
			}
		}
	}

	for prefix, cidr := range existSubnetPathMap {
		nextHop, err := m.getNextHopAddressByIP(cidr.IP)
		if err != nil {
			m.logger.Error(err, "failed to get next hop address to delete path for subnet, it will be ignore",
				"subnet", cidr.String())
			continue
		}

		if _, exist := m.subnetMap[prefix]; !exist {
			if err := m.bgpServer.DeletePath(context.Background(), &api.DeletePathRequest{
				Path: generatePathForSubnet(cidr, nextHop),
			}); err != nil {
				return fmt.Errorf("failed to delete path for subnet %v: %v", prefix, err)
			}
		}
	}

	// Ensure paths for ip instances
	for _, ipInstance := range m.ipInstanceMap {
		nextHop, err := m.getNextHopAddressByIP(ipInstance)
		if err != nil {
			m.logger.Error(err, "failed to get next hop address to add path for ip instance, it will be ignore",
				"ip", ipInstance.String())
			continue
		}

		if _, exist := existIPPathMap[ipInstance.String()]; !exist {
			if _, err := m.bgpServer.AddPath(context.Background(), &api.AddPathRequest{
				Path: generatePathForIP(ipInstance, nextHop),
			}); err != nil {
				return fmt.Errorf("failed to add path for ip instance %v: %v", ipInstance.String(), err)
			}
		}
	}

	for _, ipAddr := range existIPPathMap {
		nextHop, err := m.getNextHopAddressByIP(ipAddr)
		if err != nil {
			m.logger.Error(err, "failed to get next hop address to add path for ip instance, it will be ignore",
				"ip", ipAddr.String())
			continue
		}

		if _, exist := m.ipInstanceMap[ipAddr.String()]; !exist {
			if err := m.bgpServer.DeletePath(context.Background(), &api.DeletePathRequest{
				Path: generatePathForIP(ipAddr, nextHop),
			}); err != nil {
				return fmt.Errorf("failed to delete path for ip instance %v: %v", ipAddr.String(), err)
			}
		}
	}

	return nil
}

func (m *Manager) getNextHopAddressByIP(ipAddr net.IP) (net.IP, error) {
	if ipAddr.To4() == nil {
		if m.routerV6Address == nil {
			return nil, fmt.Errorf("router has no valid v6 nexthop address")
		}
		return m.routerV6Address, nil
	}

	if m.routerV4Address == nil {
		return nil, fmt.Errorf("router has no valid v4 nexthop address")
	}
	return m.routerV4Address, nil
}
