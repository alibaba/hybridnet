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
		return fmt.Errorf("failed to ensule %v chain and rules deleted in %v table: %v", ChainRamaPostRouting, TableNAT, err)
	}

	if err := mgr.ensureCleanBasicRuleAndChain(TableFilter, ChainRamaForward, ChainForward,
		generateRamaForwardBaseRuleSpec()...); err != nil {
		return fmt.Errorf("failed to ensule %v chain and rules deleted in %v table: %v", ChainRamaForward, TableFilter, err)
	}

	if err := mgr.ensureCleanBasicRuleAndChain(TableMangle, ChainRamaPreRouting, ChainPreRouting,
		generateRamaPreRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("failed to ensule %v chain and rules deleted in %v table: %v", ChainRamaPreRouting, TableMangle, err)
	}

	if err := mgr.ensureCleanBasicRuleAndChain(TableMangle, ChainRamaPostRouting, ChainPostRouting,
		generateRamaPostRoutingBaseRuleSpec()...); err != nil {
		return fmt.Errorf("failed to ensule %v chain and rules deleted in %v table: %v", ChainRamaPostRouting, TableMangle, err)
	}

	return nil
}

func (mgr *Manager) ensureCleanBasicRuleAndChain(table utiliptables.Table, chain, higherChain utiliptables.Chain, args ...string) error {
	// ensure base "RAMA-XXX" chains in used tables, iptables -C need it to check if rule exist
	if _, err := mgr.executor.EnsureChain(table, chain); err != nil {
		return fmt.Errorf("failed to ensule %v chain in %v table: %v", chain, table, err)
	}

	// delete base rule for "RAMA-XXX" chains
	if err := mgr.executor.DeleteRule(table, higherChain, args...); err != nil {
		return fmt.Errorf("failed to delete %v rule in %v table: %v", chain, table, err)
	}

	// flush "RAMA-XXX" chains
	if err := mgr.executor.FlushChain(table, chain); err != nil {
		return fmt.Errorf("failed to flush %v chain in %v table: %v", chain, table, err)
	}

	// delete "RAMA-XXX" chains
	if err := mgr.executor.DeleteChain(table, chain); err != nil {
		return fmt.Errorf("failed to delete %v chain in %v table: %v", chain, table, err)
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
