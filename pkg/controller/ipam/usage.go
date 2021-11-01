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

package ipam

import (
	"time"

	"k8s.io/klog"

	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/metrics"
)

type UsageSyncer struct {
	// sync period
	period time.Duration

	// stop chan
	stopEverything <-chan struct{}

	ipamManager          ipam.Interface
	dualStackIPAMManager ipam.DualStackInterface

	ipamStore          ipam.Store
	dualStackIPAMStore ipam.DualStackStore

	usageCache *Cache
}

func NewUsageSyncer(ipamManager ipam.Interface, ipamStore ipam.Store, usageCache *Cache) *UsageSyncer {
	return &UsageSyncer{
		ipamManager: ipamManager,
		ipamStore:   ipamStore,
		usageCache:  usageCache,
	}
}

func NewDualStackUsageSyncer(ipamManager ipam.DualStackInterface, ipamStore ipam.DualStackStore, usageCache *Cache) *UsageSyncer {
	return &UsageSyncer{
		dualStackIPAMManager: ipamManager,
		dualStackIPAMStore:   ipamStore,
		usageCache:           usageCache,
	}
}

func (s *UsageSyncer) Run(stopCh <-chan struct{}, period time.Duration) {
	s.period, s.stopEverything = period, stopCh

	timer := time.NewTicker(period)

	go func() {
		for {
			select {
			case <-timer.C:
				klog.V(10).Infof("periodic sync IPAM-related usage")
				s.sync()
			case <-s.stopEverything:
				timer.Stop()
				return
			}
		}
	}()

	klog.Infof("usage updater is running from %s", time.Now().String())
}

func (s *UsageSyncer) sync() {
	networks := s.usageCache.GetNetworkList()

	for _, networkName := range networks {
		if feature.DualStackEnabled() {
			nus, sus, err := s.dualStackIPAMManager.Usage(networkName)
			if err != nil {
				klog.Errorf("fail to get network %s usage: %v", networkName, err)
				continue
			}

			s.updateNetworkForDualStack(networkName, nus)
			s.updateSubnetsForDualStack(sus)

		} else {
			nu, sus, err := s.ipamManager.Usage(networkName)
			if err != nil {
				klog.Errorf("fail to get network %s usage: %v", networkName, err)
				continue
			}

			s.updateNetwork(networkName, nu)
			s.updateSubnets(sus)
		}
	}
}

// only can be used when DualStack disabled
func (s *UsageSyncer) updateNetwork(networkName string, usage *types.Usage) {
	var err error

	if s.usageCache.UpdateNetworkUsage(networkName, usage) {
		if err = s.ipamStore.SyncNetworkUsage(networkName, usage); err != nil {
			klog.Errorf("[usage-syncer] fail to sync network %s usage: %v", networkName, err)
		} else {
			klog.V(10).Infof("sync network %s with usage %+v", networkName, usage)
		}
	}
	s.updateNetworkMetrics(networkName, usage)
}

// only can be used when DualStack enabled
func (s *UsageSyncer) updateNetworkForDualStack(networkName string, usages [3]*types.Usage) {
	var err error

	if s.usageCache.UpdateNetworkUsages(networkName, usages) {
		if err = s.dualStackIPAMStore.SyncNetworkUsage(networkName, usages); err != nil {
			klog.Errorf("[usage-syncer] fail to sync network %s usages: %v", networkName, err)
		} else {
			klog.V(10).Infof("sync network %s with usage %+v", networkName, usages)
		}
	}
	// TODO: update metrics for more usages
	s.updateNetworkMetrics(networkName, usages[0])
}

func (s *UsageSyncer) updateSubnets(subnets map[string]*types.Usage) {
	var err error
	for subnetName, usage := range subnets {
		if s.usageCache.UpdateSubnetUsage(subnetName, usage) {
			if err = s.ipamStore.SyncSubnetUsage(subnetName, usage); err != nil {
				klog.Errorf("[usage-syncer] fail to sync subnet %s usage: %v", subnetName, err)
			} else {
				klog.V(10).Infof("sync subnet %s with usage %+v", subnetName, usage)
			}
		}
	}
}

func (s *UsageSyncer) updateSubnetsForDualStack(subnets map[string]*types.Usage) {
	var err error
	for subnetName, usage := range subnets {
		if s.usageCache.UpdateSubnetUsage(subnetName, usage) {
			if err = s.dualStackIPAMStore.SyncSubnetUsage(subnetName, usage); err != nil {
				klog.Errorf("[usage-syncer] fail to sync subnet %s usage: %v", subnetName, err)
			} else {
				klog.V(10).Infof("sync subnet %s with usage %+v", subnetName, usage)
			}
		}
	}
}

func (s *UsageSyncer) updateNetworkMetrics(networkName string, networkUsage *types.Usage) {
	metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPTotalUsageType).Set(float64(networkUsage.Total))
	metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPUsedUsageType).Set(float64(networkUsage.Used))
	metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPAvailableUsageType).Set(float64(networkUsage.Available))
}
