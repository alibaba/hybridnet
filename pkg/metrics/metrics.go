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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	IPTotalUsageType     = "total"
	IPUsedUsageType      = "used"
	IPAvailableUsageType = "available"
)

const (
	IPv4      = "ipv4"
	IPv6      = "ipv6"
	DualStack = "dualstack"
)

var IPUsageGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "ip_usage",
		Help: "the usage of IPs in different networks",
	},
	[]string{
		"networkName",
		"ipFamily",
		"usageType",
	},
)

const (
	IPStatefulAllocateType = "stateful"
	IPNormalAllocateType   = "normal"
)

var IPAllocationPeriodSummary = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "ip_allocation_period",
		Help:       "the period summary of ip allocation for pod",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{
		"allocateType",
		"success",
	},
)

var RemoteClusterStatusCheckDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "remote_cluster_status_check_duration",
		Help:    "time taken for checking remote cluster status.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.5, 5.0, 7.5, 10.0, 12.5, 15.0, 17.5, 20.0, 22.5, 25.0, 27.5, 30.0, 50.0, 75.0, 100.0, 1000.0},
	},
	[]string{
		"clusterName",
	},
)

func RegisterForManager() prometheus.Gatherer {
	r := prometheus.NewRegistry()
	r.MustRegister(IPUsageGauge)
	r.MustRegister(IPAllocationPeriodSummary)
	r.MustRegister(RemoteClusterStatusCheckDuration)
	return r
}
