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
	"sync"

	"google.golang.org/protobuf/types/known/anypb"

	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"

	"github.com/vishvananda/netlink"

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

	peerMap   map[string]*peerInfo
	subnetMap map[string]*net.IPNet
	ipMap     map[string]*ipInfo

	startMutex sync.RWMutex
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

		peerMap:   map[string]*peerInfo{},
		subnetMap: map[string]*net.IPNet{},
		ipMap:     map[string]*ipInfo{},

		startMutex: sync.RWMutex{},
	}

	peeringLink, err := netlink.LinkByName(peeringInterfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bgp peering link %v: %v", peeringInterfaceName, err)
	}

	existLinkAddress, err := daemonutils.ListAllAddress(peeringLink)
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
		if (existLinkAddress[0].IP.To4() == nil) != (existLinkAddress[1].IP.To4() == nil) {
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
		defaultV4Route, err := daemonutils.GetDefaultRoute(netlink.FAMILY_V4)
		if err != nil && err != daemonutils.NotExist {
			return nil, fmt.Errorf("failed to get v4 default route: %v", err)
		}

		defaultV6Route, err := daemonutils.GetDefaultRoute(netlink.FAMILY_V6)
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

func (m *Manager) RecordIP(ip net.IP, needToBeExported bool) {
	m.ipMap[ip.String()] = &ipInfo{
		ip:               ip,
		needToBeExported: needToBeExported,
	}
}

func (m *Manager) ResetSubnetInfos() {
	m.subnetMap = map[string]*net.IPNet{}
}

func (m *Manager) ResetPeerInfos() {
	m.peerMap = map[string]*peerInfo{}
}

func (m *Manager) ResetPeerAndSubnetInfos() {
	m.ResetPeerInfos()
	m.ResetSubnetInfos()
}

func (m *Manager) ResetIPInfos() {
	m.ipMap = map[string]*ipInfo{}
}

func (m *Manager) TryStart(asn uint32) error {
	m.startMutex.Lock()
	defer m.startMutex.Unlock()

	if m.localASN != 0 {
		if m.localASN != asn {
			return fmt.Errorf("can not restart bgp manager (local AS number: %v) with a different AS number %v",
				m.localASN, asn)
		}
		return nil
	}

	m.localASN = asn
	return m.bgpServer.StartBgp(context.Background(), &api.StartBgpRequest{
		Global: &api.Global{
			Asn:      m.localASN,
			RouterId: m.routerID,
		},
	})
}

func (m *Manager) CheckIfStart() bool {
	m.startMutex.Lock()
	defer m.startMutex.Unlock()

	return m.localASN != 0
}

func (m *Manager) SyncPeerInfos() error {
	// If bgp manager is not started, do nothing.
	if !m.CheckIfStart() {
		return nil
	}

	// Sync peers configuration.
	// Because now UpdatePeer will reset bgp session causing a network fluctuation, we will never update an exist bgp peer.
	existPeerMap := map[string]struct{}{}
	if err := m.listRemoteBGPPeers(existPeerMap, func(peer *api.Peer) bool {
		return true
	}); err != nil {
		return fmt.Errorf("failed to list all bgp peers: %v", err)
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

	for addr := range existPeerMap {
		if _, exist := m.peerMap[addr]; !exist {
			if err := m.bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{
				Address: addr,
			}); err != nil {
				return fmt.Errorf("failed to add bgp peer %v: %v", addr, err)
			}
		}
	}

	return nil
}

func (m *Manager) SyncSubnetInfos() error {
	// If bgp manager is not started, do nothing.
	if !m.CheckIfStart() {
		return nil
	}

	// Sync subnet paths.
	existSubnetPathMap := map[string]*net.IPNet{}
	if err := m.listExistPath(existSubnetPathMap, nil); err != nil {
		return fmt.Errorf("failed to list exist subnet paths: %v", err)
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

	return nil
}

func (m *Manager) SyncPeerAndSubnetInfos() error {
	if err := m.SyncPeerInfos(); err != nil {
		return err
	}

	return m.SyncSubnetInfos()
}

func (m *Manager) SyncIPInfos() error {
	// If bgp manager is not started, do nothing.
	if !m.CheckIfStart() {
		return nil
	}

	existIPPathMap := map[string]net.IP{}
	if err := m.listExistPath(nil, existIPPathMap); err != nil {
		return fmt.Errorf("failed to list exist ip paths: %v", err)
	}

	// Ensure paths for ip instances
	for _, ipInstance := range m.ipMap {
		nextHop, err := m.getNextHopAddressByIP(ipInstance.ip)
		if err != nil {
			m.logger.Error(err, "failed to get next hop address to add path for ip instance, it will be ignore",
				"ip", ipInstance.ip.String())
			continue
		}

		var extraPathAttrs []*anypb.Any
		if !ipInstance.needToBeExported {
			extraPathAttrs = append(extraPathAttrs, noExportCommunityAttr)
		}

		if _, exist := existIPPathMap[ipInstance.ip.String()]; !exist {
			if _, err := m.bgpServer.AddPath(context.Background(), &api.AddPathRequest{
				Path: generatePathForIP(ipInstance.ip, nextHop, extraPathAttrs...),
			}); err != nil {
				return fmt.Errorf("failed to add path for ip instance %v: %v", ipInstance.ip.String(), err)
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

		if _, exist := m.ipMap[ipAddr.String()]; !exist {
			// delete path don't need attrs
			if err := m.bgpServer.DeletePath(context.Background(), &api.DeletePathRequest{
				Path: generatePathForIP(ipAddr, nextHop),
			}); err != nil {
				return fmt.Errorf("failed to delete path for ip instance %v: %v", ipAddr.String(), err)
			}
		}
	}

	return nil
}

func (m *Manager) CheckIfIPInfoPathAdded(ipAddr net.IP) (bool, error) {
	existIPPathMap := map[string]net.IP{}
	if err := m.listExistPath(nil, existIPPathMap); err != nil {
		return false, fmt.Errorf("failed to list exist ip paths: %v", err)
	}

	_, exist := existIPPathMap[ipAddr.String()]
	return exist, nil
}

func (m *Manager) CheckEstablishedRemotePeerExists() (bool, error) {
	establishedPeerMap := map[string]struct{}{}
	if err := m.listRemoteBGPPeers(establishedPeerMap, func(peer *api.Peer) bool {
		return peer.State.SessionState == api.PeerState_ESTABLISHED
	}); err != nil {
		return false, fmt.Errorf("failed to list all the established bgp peers: %v", err)
	}

	if len(establishedPeerMap) == 0 {
		return false, nil
	}
	return true, nil
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

func (m *Manager) listExistPath(existSubnetPathMap map[string]*net.IPNet, existIPPathMap map[string]net.IP) error {
	listPathFunc := generatePathListFunc(existSubnetPathMap, existIPPathMap, m.logger)
	if err := m.bgpServer.ListPath(context.Background(),
		&api.ListPathRequest{Family: v4Family}, listPathFunc); err != nil {
		return fmt.Errorf("failed to list ipv4 path: %v", err)
	}
	if err := m.bgpServer.ListPath(context.Background(),
		&api.ListPathRequest{Family: v6Family}, listPathFunc); err != nil {
		return fmt.Errorf("failed to list ipv6 path: %v", err)
	}
	return nil
}

func (m *Manager) listRemoteBGPPeers(existPeerMap map[string]struct{}, filterFunc func(peer *api.Peer) bool) error {
	if err := m.bgpServer.ListPeer(context.Background(), &api.ListPeerRequest{EnableAdvertised: true},
		func(peer *api.Peer) {
			if filterFunc(peer) {
				existPeerMap[peer.Conf.NeighborAddress] = struct{}{}
			}
		}); err != nil {
		return fmt.Errorf("failed to list bgp peers: %v", err)
	}
	return nil
}
