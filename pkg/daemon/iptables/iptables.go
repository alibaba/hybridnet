/*
Copyright 2021 The Rama Authors.

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

	extraliptables "github.com/coreos/go-iptables/iptables"
	"github.com/gogf/gf/container/gset"
	"github.com/oecp/rama/pkg/daemon/ipset"
	daemonutils "github.com/oecp/rama/pkg/daemon/utils"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	"k8s.io/utils/exec"
)

const (
	TableNAT    = "nat"
	TableFilter = "filter"
	TableMangle = "mangle"

	ChainPostRouting = "POSTROUTING"
	ChainPreRouting  = "PREROUTING"

	ChainRamaPostRouting = "RAMA-POSTROUTING"
	ChainRamaForward     = "RAMA-FORWARD"
	ChainRamaPreRouting  = "RAMA-PREROUTING"
	ChainForward         = "FORWARD"

	RamaOverlayNetSetName = "RAMA-OVERLAY-NET"
	RamaAllIPSetName      = "RAMA-ALL"
	RamaNodeIPSetName     = "RAMA-NODE-IP"

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
	localCidr      *gset.StrSet

	overlayIfName string

	protocol Protocol

	c chan struct{}

	// add cluster-mesh remote ips
	remoteOverlaySubnet  []*net.IPNet
	remoteUnderlaySubnet []*net.IPNet
	remoteNodeIPList     []net.IP
	remoteSubnetTracker  *daemonutils.SubnetCidrTracker
	remoteCidr           *gset.StrSet
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
		localCidr:      gset.NewStrSet(),

		protocol: protocol,
		c:        make(chan struct{}, 1),

		remoteOverlaySubnet:  []*net.IPNet{},
		remoteUnderlaySubnet: []*net.IPNet{},
		remoteNodeIPList:     []net.IP{},
		remoteSubnetTracker:  daemonutils.NewSubnetCidrTracker(),
		remoteCidr:           gset.NewStrSet(),
	}

	return mgr, nil
}

func (mgr *Manager) Reset() {
	mgr.overlaySubnet = []*net.IPNet{}
	mgr.underlaySubnet = []*net.IPNet{}
	mgr.nodeIPList = []net.IP{}
	mgr.localCidr.Clear()
	mgr.overlayIfName = ""
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

	mgr.localCidr.Add(subnetCidr.String())
}

func (mgr *Manager) SetOverlayIfName(overlayIfName string) {
	mgr.overlayIfName = overlayIfName
}

func (mgr *Manager) SyncRules() error {
	mgr.lock()
	defer mgr.unlock()

	// check out remote subnet configurations
	configRemote, rcErr := mgr.configureRemote()
	if rcErr != nil {
		return fmt.Errorf("iptables manager detects illegal remote subnet config: %v", rcErr)
	}

	if mgr.overlayIfName == "" {
		return fmt.Errorf("cannot sync iptables rules with empty overlay interface name")
	}

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

	if configRemote {
		for _, cidr := range mgr.remoteOverlaySubnet {
			overlayIPNets = append(overlayIPNets, cidr.String())
		}
		for _, cidr := range mgr.remoteUnderlaySubnet {
			allIPNets = append(allIPNets, cidr.String())
		}
		for _, ip := range mgr.remoteNodeIPList {
			nodeIPs = append(nodeIPs, ip.String())
		}
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

	ipsetInterface.AddOrReplaceIPSet(generateIPSetNameByProtocol(RamaOverlayNetSetName, mgr.protocol),
		overlayIPNets, ipset.TypeHashNet, ipset.OptionTimeout, "0")
	ipsetInterface.AddOrReplaceIPSet(generateIPSetNameByProtocol(RamaAllIPSetName, mgr.protocol),
		allIPNets, ipset.TypeHashNet, ipset.OptionTimeout, "0")
	ipsetInterface.AddOrReplaceIPSet(generateIPSetNameByProtocol(RamaNodeIPSetName, mgr.protocol),
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

	writeLine(natChains, utiliptables.MakeChainLine(ChainRamaPostRouting))
	writeLine(filterChains, utiliptables.MakeChainLine(ChainRamaForward))
	writeLine(mangleChains, utiliptables.MakeChainLine(ChainRamaPreRouting))
	writeLine(mangleChains, utiliptables.MakeChainLine(ChainRamaPostRouting))

	// Append rules.
	writeLine(natRules, generateMasqueradeRuleSpec(mgr.overlayIfName, mgr.protocol)...)
	writeLine(filterRules, generateVxlanFilterRuleSpec(mgr.overlayIfName, mgr.protocol)...)
	writeLine(mangleRules, generateVxlanPodToNodeReplyMarkRuleSpec(mgr.protocol)...)
	writeLine(mangleRules, generateVxlanPodToNodeReplyRemoveMarkRuleSpec(mgr.protocol)...)

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
		return fmt.Errorf("Failed to execute iptables-restore: " + err.Error() +
			"\n iptables rules are:\n " + iptablesData.String())
	}

	return nil
}

func (mgr *Manager) ensureBasicRuleAndChains() error {
	// ensure base chain and rule for RAMA-POSTROUTING in nat table
	if _, err := mgr.executor.EnsureChain(TableNAT, ChainRamaPostRouting); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainRamaPostRouting, TableNAT, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableNAT, ChainPostRouting,
		generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaPostRouting, TableNAT, err)
	}

	// ensure base chain and rule for RAMA-FORWARD in filter table
	if _, err := mgr.executor.EnsureChain(TableFilter, ChainRamaForward); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainRamaForward, TableFilter, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableFilter, ChainForward,
		generateRamaForwardBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaForward, TableFilter, err)
	}

	// ensure base chain and rule for RAMA-PREROUTING in mangle table
	if _, err := mgr.executor.EnsureChain(TableMangle, ChainRamaPreRouting); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainRamaPreRouting, TableMangle, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableMangle, ChainPreRouting,
		generateRamaPreRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaPreRouting, TableMangle, err)
	}

	// ensure base chain and rule for RAMA-POSTROUTING in mangle table
	if _, err := mgr.executor.EnsureChain(TableMangle, ChainRamaPostRouting); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", ChainRamaPostRouting, TableMangle, err)
	}

	if _, err := mgr.executor.EnsureRule(utiliptables.Append, TableMangle, ChainPostRouting,
		generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaPostRouting, TableMangle, err)
	}

	return nil
}

func generateIPSetNameByProtocol(setBaseName string, protocol Protocol) string {
	if protocol == ProtocolIpv4 {
		return setBaseName + "-V4"
	}
	return setBaseName + "-V6"
}

func generateRamaPostRoutingBaseRuleSpec() []string {
	return []string{"-m", "comment", "--comment", "rama postrouting rules", "-j", ChainRamaPostRouting}
}

func generateRamaForwardBaseRuleSpec() []string {
	return []string{"-m", "comment", "--comment", "rama forward rules", "-j", ChainRamaForward}
}

func generateRamaPreRoutingBaseRuleSpec() []string {
	return []string{"-m", "comment", "--comment", "rama prerouting rules", "-j", ChainRamaPreRouting}
}

func generateMasqueradeRuleSpec(vxlanIf string, protocol Protocol) []string {
	return []string{"-A", ChainRamaPostRouting, "-m", "comment", "--comment", `"rama overlay nat-outgoing masquerade rule"`,
		"!", "-o", vxlanIf, "-m", "set", "--match-set", generateIPSetNameByProtocol(RamaOverlayNetSetName, protocol),
		"src", "-j", "MASQUERADE"}
}

func generateVxlanFilterRuleSpec(vxlanIf string, protocol Protocol) []string {
	return []string{"-A", ChainRamaForward, "-m", "comment", "--comment", `"rama overlay vxlan if egress filter rule"`,
		"-o", vxlanIf, "-m", "set", "!", "--match-set", generateIPSetNameByProtocol(RamaAllIPSetName, protocol),
		"dst", "-j", "REJECT", "--reject-with", rejectWithOption(protocol)}
}

func generateVxlanPodToNodeReplyMarkRuleSpec(protocol Protocol) []string {
	return []string{"-A", ChainRamaPreRouting, "-m", "comment", "--comment", `"mark overlay pod -> node back traffic"`,
		"-m", "addrtype", "!", "--dst-type", "LOCAL",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(RamaOverlayNetSetName, protocol), "src",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(RamaNodeIPSetName, protocol), "dst",
		"-m", "conntrack", "!", "--ctstate", "NEW,INVALID,DNAT,SNAT",
		"-j", "MARK", "--set-xmark", fmt.Sprintf("%s/%s", PodToNodeBackTrafficMarkString, PodToNodeBackTrafficMarkString),
	}
}

func generateVxlanPodToNodeReplyRemoveMarkRuleSpec(protocol Protocol) []string {
	return []string{"-A", ChainRamaPostRouting, "-m", "comment", "--comment", `"remove overlay pod -> node back traffic mark"`,
		"-m", "addrtype", "!", "--dst-type", "LOCAL",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(RamaOverlayNetSetName, protocol), "src",
		"-m", "set", "--match-set", generateIPSetNameByProtocol(RamaNodeIPSetName, protocol), "dst",
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
