package iptables

import (
	"fmt"
)

const (
	ChainRamaPostRouting = "RAMA-POSTROUTING"
	ChainRamaForward     = "RAMA-FORWARD"
	ChainRamaPreRouting  = "RAMA-PREROUTING"
)

func (mgr *Manager) cleanDeprecatedBasicRules() error {
	// delete base rule for RAMA-POSTROUTING in nat table
	if err := mgr.executor.DeleteRule(TableNAT, ChainPostRouting, generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaPostRouting, TableNAT, err)
	}

	// delete base rule for RAMA-FORWARD in filter table
	if err := mgr.executor.DeleteRule(TableFilter, ChainForward, generateRamaForwardBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaForward, TableFilter, err)
	}

	// delete base rule for RAMA-PREROUTING in mangle table
	if err := mgr.executor.DeleteRule(TableMangle, ChainPreRouting, generateRamaPreRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaPreRouting, TableMangle, err)
	}

	// delete base rule for RAMA-POSTROUTING in mangle table
	if err := mgr.executor.DeleteRule(TableMangle, ChainPostRouting, generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("ensure %v rule in %v table failed: %v", ChainRamaPostRouting, TableMangle, err)
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
