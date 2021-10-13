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

package iptables

import (
	"bytes"
	"fmt"
	"net"

	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"

	"github.com/alibaba/hybridnet/pkg/daemon/ipset"
	extraliptables "github.com/coreos/go-iptables/iptables"

	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	"k8s.io/utils/exec"
)

const (
	TableNAT    = "nat"
	TableFilter = "filter"
	TableMangle = "mangle"

	ChainPostRouting = "POSTROUTING"
	ChainPreRouting  = "PREROUTING"
	ChainForward     = "FORWARD"

	ChainHybridnetPostRouting = "HYBRIDNET-POSTROUTING"
	ChainHybridnetForward     = "HYBRIDNET-FORWARD"
	ChainHybridnetPreRouting  = "HYBRIDNET-PREROUTING"

	HybridnetOverlayNetSetName = "HYBRIDNET-OVERLAY-NET"
	HybridnetAllIPSetName      = "HYBRIDNET-ALL"
	HybridnetNodeIPSetName     = "HYBRIDNET-NODE-IP"

	PodToNodeBackTrafficMarkString = "0x20"
	PodToNodeBackTrafficMark       = 0x20
)

// Protocol defines the ip protocol either ipv4 or ipv6
type Protocol byte

const (
	// ProtocolIpv4 represents ipv4 protocol in iptables
	ProtocolIpv4 Protocol = iota + 1
	// ProtocolIpv6 represents ipv6 protocol in iptables
	ProtocolIpv6
)

type Manager struct {
	executor utiliptables.Interface
	helper   *extraliptables.IPTables

	overlaySubnet  []*net.IPNet
	underlaySubnet []*net.IPNet
	nodeIPList     []net.IP

	overlayIfName string

	protocol Protocol

	c chan struct{}

	upgradeWorkDone bool

	// add cluster-mesh remote ips
	remoteOverlaySubnet  []*net.IPNet
	remoteUnderlaySubnet []*net.IPNet
	remoteNodeIPList     []net.IP
}

func (mgr *Manager) lock() {
	mgr.c <- struct{}{}
}

func (mgr *Manager) unlock() {
	<-mgr.c
}

func CreateIPtablesManager(protocol Protocol) (*Manager, error) {
	// Create a iptables utils.
	execer := exec.New()

	var interfaceProtocol utiliptables.Protocol
	var helperProtocol extraliptables.Protocol
	var err error

	switch protocol {
	case ProtocolIpv4:
		interfaceProtocol = utiliptables.ProtocolIpv4
		helperProtocol = extraliptables.ProtocolIPv4
	case ProtocolIpv6:
		interfaceProtocol = utiliptables.ProtocolIpv6
		helperProtocol = extraliptables.ProtocolIPv6
	default:
		return nil, fmt.Errorf("iptables version %v not supported", protocol)
	}

	iptInterface := utiliptables.New(execer, interfaceProtocol)

	helper, err := extraliptables.NewWithProtocol(helperProtocol)
	if err != nil {
		return nil, fmt.Errorf("create iptables manager error: %v", err)
	}

	mgr := &Manager{
		executor: iptInterface,
		helper:   helper,

		overlaySubnet:  []*net.IPNet{},
		underlaySubnet: []*net.IPNet{},
		nodeIPList:     []net.IP{},

		protocol: protocol,
		c:        make(chan struct{}, 1),

		remoteOverlaySubnet:  []*net.IPNet{},
		remoteUnderlaySubnet: []*net.IPNet{},
		remoteNodeIPList:     []net.IP{},
	}

	return mgr, nil
}

func (mgr *Manager) Reset() {
	mgr.overlaySubnet = []*net.IPNet{}
	mgr.underlaySubnet = []*net.IPNet{}
	mgr.nodeIPList = []net.IP{}
	mgr.overlayIfName = ""

	mgr.remoteOverlaySubnet = []*net.IPNet{}
	mgr.remoteUnderlaySubnet = []*net.IPNet{}
	mgr.remoteNodeIPList = []net.IP{}
}

func (mgr *Manager) RecordNodeIP(nodeIP net.IP) {
	mgr.nodeIPList = append(mgr.nodeIPList, nodeIP)
}

func (mgr *Manager) RecordSubnet(subnetCidr *net.IPNet, isOverlay bool) {
	if isOverlay {
		mgr.overlaySubnet = append(mgr.overlaySubnet, subnetCidr)
	} else {
		mgr.underlaySubnet = append(mgr.underlaySubnet, subnetCidr)
	}
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

func (mgr *Manager) SetOverlayIfName(overlayIfName string) {
	mgr.overlayIfName = overlayIfName
}

func (mgr *Manager) SyncRules() error {
	mgr.lock()
	defer mgr.unlock()

	var overlayIPNets []string
	var nodeIPs []string

	for _, cidr := range mgr.overlaySubnet {
		overlayIPNets = append(overlayIPNets, cidr.String())
	}

	var allIPNets []string
	for _, cidr := range mgr.underlaySubnet {
		allIPNets = append(allIPNets, cidr.String())
	}

	for _, ip := range mgr.nodeIPList {
		nodeIPs = append(nodeIPs, ip.String())
	}

	// remote subnets & nodes
	for _, cidr := range mgr.remoteOverlaySubnet {
		overlayIPNets = append(overlayIPNets, cidr.String())
	}
	for _, cidr := range mgr.remoteUnderlaySubnet {
		allIPNets = append(allIPNets, cidr.String())
	}
	for _, ip := range mgr.remoteNodeIPList {
		nodeIPs = append(nodeIPs, ip.String())
	}

	allIPNets = append(allIPNets, overlayIPNets...)
	allIPNets = append(allIPNets, nodeIPs...)

	ipsetInterface, err := ipset.New(mgr.protocol == ProtocolIpv6)
	if err != nil {
		return fmt.Errorf("failed to create ipset instance: %v", err)
	}

	if err := ipsetInterface.LoadData(); err != nil {
		return fmt.Errorf("failed to load ipset data: %v", err)
	}

	ipsetInterface.AddOrReplaceIPSet(generateIPSetNameByProtocol(HybridnetOverlayNetSetName, mgr.protocol),
		overlayIPNets, ipset.TypeHashNet, ipset.OptionTimeout, "0")
	ipsetInterface.AddOrReplaceIPSet(generateIPSetNameByProtocol(HybridnetAllIPSetName, mgr.protocol),
		allIPNets, ipset.TypeHashNet, ipset.OptionTimeout, "0")
	ipsetInterface.AddOrReplaceIPSet(generateIPSetNameByProtocol(HybridnetNodeIPSetName, mgr.protocol),
		nodeIPs, ipset.TypeHashIP, ipset.OptionTimeout, "0")

	if err := mgr.ensureBasicRuleAndChains(); err != nil {
		return fmt.Errorf("ensure basic rules and chains failed: %v", err)
	}

	iptablesData := bytes.NewBuffer(nil)
	filterChains := bytes.NewBuffer(nil)
	filterRules := bytes.NewBuffer(nil)
	natChains := bytes.NewBuffer(nil)
	natRules := bytes.NewBuffer(nil)
	mangleChains := bytes.NewBuffer(nil)
	mangleRules := bytes.NewBuffer(nil)

	// Write table headers.
	writeLine(natChains, "*nat")
	writeLine(filterChains, "*filter")
	writeLine(mangleChains, "*mangle")

	writeLine(natChains, utiliptables.MakeChainLine(ChainHybridnetPostRouting))
	writeLine(filterChains, utiliptables.MakeChainLine(ChainHybridnetForward))
	writeLine(mangleChains, utiliptables.MakeChainLine(ChainHybridnetPreRouting))
	writeLine(mangleChains, utiliptables.MakeChainLine(ChainHybridnetPostRouting))

	if mgr.overlayIfName != "" {
		// There might be two scenarios where overlayIfName is nil
		// 1. overlay network never exists
		// 2. overlay network deleted after running for a period
		//
		// Keep iptables chains empty for both two scenarios.
		//
		// Append rules.
		writeLine(natRules, generateSkipMasqueradeRuleSpec()...)
		writeLine(natRules, generateMasqueradeRuleSpec(mgr.overlayIfName, mgr.protocol)...)
		writeLine(filterRules, generateVxlanFilterRuleSpec(mgr.overlayIfName, mgr.protocol)...)
		writeLine(mangleRules, generateVxlanPodToNodeReplyMarkRuleSpec(mgr.protocol)...)
		writeLine(mangleRules, generateVxlanPodToNodeReplyRemoveMarkRuleSpec(mgr.protocol)...)
	}

	// Write the end-of-table markers
	writeLine(natRules, "COMMIT")
	writeLine(filterRules, "COMMIT")
	writeLine(mangleRules, "COMMIT")

	// Sync ipsets
	if err := ipsetInterface.SyncOperations(); err != nil {
		return fmt.Errorf("failed to execute sync ipset operations: %v", err)
	}

	// Sync rules
	iptablesData.Write(natChains.Bytes())
	iptablesData.Write(natRules.Bytes())
	iptablesData.Write(filterChains.Bytes())
	iptablesData.Write(filterRules.Bytes())
	iptablesData.Write(mangleChains.Bytes())
	iptablesData.Write(mangleRules.Bytes())

	if err := mgr.executor.RestoreAll(iptablesData.Bytes(), utiliptables.NoFlushTables,
		utiliptables.RestoreCounters); err != nil {
		return fmt.Errorf("failed to execute iptables-restore: " + err.Error() +
			"\n iptables rules are:\n " + iptablesData.String())
	}

	// TODO: update logic, need to be removed further
	if !mgr.upgradeWorkDone {
		if err := mgr.cleanDeprecatedBasicRuleAndChains(); err != nil {
			return fmt.Errorf("failed to clean deprecated basic rules: %v", err)
		}
		mgr.upgradeWorkDone = true
	}

	return nil
}

func (mgr *Manager) ensureBasicRuleAndChains() error {
	// ensure base chain and rule for HYBRIDNET-POSTROUTING in nat table
	if _, err := mgr.executor.EnsureChain(TableNAT, ChainHybridnetPostRouting); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainHybridnetPostRouting, TableNAT, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableNAT, ChainPostRouting,
		generateHybridnetPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainHybridnetPostRouting, TableNAT, err)
	}

	// ensure base chain and rule for HYBRIDNET-FORWARD in filter table
	if _, err := mgr.executor.EnsureChain(TableFilter, ChainHybridnetForward); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainHybridnetForward, TableFilter, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableFilter, ChainForward,
		generateHybridnetForwardBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainHybridnetForward, TableFilter, err)
	}

	// ensure base chain and rule for HYBRIDNET-PREROUTING in mangle table
	if _, err := mgr.executor.EnsureChain(TableMangle, ChainHybridnetPreRouting); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainHybridnetPreRouting, TableMangle, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableMangle, ChainPreRouting,
		generateHybridnetPreRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainHybridnetPreRouting, TableMangle, err)
	}

	// ensure base chain and rule for HYBRIDNET-POSTROUTING in mangle table
	if _, err := mgr.executor.EnsureChain(TableMangle, ChainHybridnetPostRouting); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainHybridnetPostRouting, TableMangle, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableMangle, ChainPostRouting,
		generateHybridnetPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainHybridnetPostRouting, TableMangle, err)
	}

	return nil
}

func generateIPSetNameByProtocol(setBaseName string, protocol Protocol) string {
	if protocol == ProtocolIpv4 {
		return setBaseName + "-V4"
	}
	return setBaseName + "-V6"
}

func generateHybridnetPostRoutingBaseRuleSpec() []string {
	return []string{"-m", "comment", "--comment", "hybridnet postrouting rules", "-j", ChainHybridnetPostRouting}
}

func generateHybridnetForwardBaseRuleSpec() []string {
	return []string{"-m", "comment", "--comment", "hybridnet forward rules", "-j", ChainHybridnetForward}
}

func generateHybridnetPreRoutingBaseRuleSpec() []string {
	return []string{"-m", "comment", "--comment", "hybridnet prerouting rules", "-j", ChainHybridnetPreRouting}
}

func generateMasqueradeRuleSpec(vxlanIf string, protocol Protocol) []string {
	return []string{"-A", ChainHybridnetPostRouting, "-m", "comment", "--comment", `"hybridnet overlay nat-outgoing masquerade rule"`,
		"!", "-o", vxlanIf, "-m", "set", "--match-set", generateIPSetNameByProtocol(HybridnetOverlayNetSetName, protocol),
		"src", "-j", "MASQUERADE"}
}

func generateSkipMasqueradeRuleSpec() []string {
	return []string{"-A", ChainHybridnetPostRouting, "-m", "comment", "--comment", `"skip masquerade if traffic is to local pod"`,
		"-o", containernetwork.ContainerHostLinkPrefix + "+", "-j", "RETURN"}
}

func generateVxlanFilterRuleSpec(vxlanIf string, protocol Protocol) []string {
	return []string{"-A", ChainHybridnetForward, "-m", "comment", "--comment", `"hybridnet overlay vxlan if egress filter rule"`,
		"-o", vxlanIf, "-m", "set", "!", "--match-set", generateIPSetNameByProtocol(HybridnetAllIPSetName, protocol),
		"dst", "-j", "REJECT", "--reject-with", rejectWithOption(protocol)}
}

func generateVxlanPodToNodeReplyMarkRuleSpec(protocol Protocol) []string {
	return []string{"-A", ChainHybridnetPreRouting, "-m", "comment", "--comment", `"mark overlay pod -> node back traffic"`,
		"-m", "addrtype", "!", "--dst-type", "LOCAL",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(HybridnetOverlayNetSetName, protocol), "src",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(HybridnetNodeIPSetName, protocol), "dst",
		"-m", "conntrack", "!", "--ctstate", "NEW,INVALID,DNAT,SNAT",
		"-j", "MARK", "--set-xmark", fmt.Sprintf("%s/%s", PodToNodeBackTrafficMarkString, PodToNodeBackTrafficMarkString),
	}
}

func generateVxlanPodToNodeReplyRemoveMarkRuleSpec(protocol Protocol) []string {
	return []string{"-A", ChainHybridnetPostRouting, "-m", "comment", "--comment", `"remove overlay pod -> node back traffic mark"`,
		"-m", "addrtype", "!", "--dst-type", "LOCAL",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(HybridnetOverlayNetSetName, protocol), "src",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(HybridnetNodeIPSetName, protocol), "dst",
		"-m", "conntrack", "!", "--ctstate", "NEW,INVALID,DNAT,SNAT",
		"-j", "MARK", "--set-xmark", fmt.Sprintf("0x0/%s", PodToNodeBackTrafficMarkString),
	}
}

func rejectWithOption(protocol Protocol) string {
	if protocol == ProtocolIpv4 {
		return "icmp-host-unreachable"
	}
	return "icmp6-addr-unreachable"
}
