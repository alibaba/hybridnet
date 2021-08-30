package iptables

import (
	"fmt"

	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
)

const (
	ChainRamaPostRouting = "RAMA-POSTROUTING"
	ChainRamaForward     = "RAMA-FORWARD"
	ChainRamaPreRouting  = "RAMA-PREROUTING"
)

func (mgr *Manager) cleanDeprecatedBasicRuleAndChains() error {
	if err := mgr.ensureCleanBasicRuleAndChain(TableNAT, ChainRamaPostRouting, ChainPostRouting,
		generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensule %v chain and rules deleted in %v table failed: %v", ChainRamaPostRouting, TableNAT, err)
	}

	if err := mgr.ensureCleanBasicRuleAndChain(TableFilter, ChainRamaForward, ChainForward,
		generateRamaForwardBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensule %v chain and rules deleted in %v table failed: %v", ChainRamaForward, TableFilter, err)
	}

	if err := mgr.ensureCleanBasicRuleAndChain(TableMangle, ChainRamaPreRouting, ChainPreRouting,
		generateRamaPreRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensule %v chain and rules deleted in %v table failed: %v", ChainRamaPreRouting, TableMangle, err)
	}

	if err := mgr.ensureCleanBasicRuleAndChain(TableMangle, ChainRamaPostRouting, ChainPostRouting,
		generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensule %v chain and rules deleted in %v table failed: %v", ChainRamaPostRouting, TableMangle, err)
	}

	return nil
}

func (mgr *Manager) ensureCleanBasicRuleAndChain(table utiliptables.Table, chain, higherChain utiliptables.Chain, args ...string) error {
	// ensure base "RAMA-XXX" chains in used tables, iptables -C need it to check if rule exist
	if _, err := mgr.executor.EnsureChain(table, chain); err != nil {
		return fmt.Errorf("ensule %v chain in %v table failed: %v", chain, table, err)
	}

	// delete base rule for "RAMA-XXX" chains
	if err := mgr.executor.DeleteRule(table, higherChain, args...); err != nil {
		return fmt.Errorf("delete %v rule in %v table failed: %v", chain, table, err)
	}

	// flush "RAMA-XXX" chains
	if err := mgr.executor.FlushChain(table, chain); err != nil {
		return fmt.Errorf("flush %v chain in %v table failed: %v", chain, table, err)
	}

	// delete "RAMA-XXX" chains
	if err := mgr.executor.DeleteChain(table, chain); err != nil {
		return fmt.Errorf("delete %v chain in %v table failed: %v", chain, table, err)
	}

	return nil
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
