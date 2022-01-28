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

package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/alibaba/hybridnet/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	entryLog = log.Log.WithName("entry")

	metricsPort = pflag.String("metrics-port", "9899", "The port to listen on for prometheus metrics.")

	gather prometheus.Gatherer
)

func init() {
	gather = metrics.RegisterForManager()
}

func startMetricsServer() {
	http.Handle("/metrics", promhttp.HandlerFor(
		gather,
		promhttp.HandlerOpts{},
	))

	entryLog.Info("Starting metrics server", "time", time.Now().String())

	entryLog.Error(http.ListenAndServe(fmt.Sprintf(":%s", *metricsPort), nil),
		"failed make metrics server listen and serve", "metrics-port", *metricsPort)
}
