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

package main

import (
	"context"
	"flag"

	"github.com/spf13/pflag"
	"k8s.io/klog"
	controllerruntime "sigs.k8s.io/controller-runtime"

	"github.com/oecp/rama/pkg/feature"
	"github.com/oecp/rama/pkg/manager"
)

var (
	gitCommit       string
	defaultIPRetain bool
)

func init() {
	klog.InitFlags(nil)

	pflag.BoolVar(&defaultIPRetain, "default-ip-retain", true, "Whether pod IP of stateful workloads will be retained by default.")
}

func main() {
	// parse flags
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	klog.Infof("Starting rama manager with git commit: %v", gitCommit)
	klog.Infof("known features: %v", feature.KnownFeatures())

	var (
		stopCh <-chan struct{}
		mgr    *manager.Manager
		err    error
	)

	// init stop channel & context
	stopCh = controllerruntime.SetupSignalHandler()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-stopCh
		cancel()
	}()

	// setup metrics http server
	go startMetricsServer()

	mgr, err = manager.NewManager(manager.Config{
		DefaultRetainIP: defaultIPRetain,
	})
	if err != nil {
		klog.Fatalf("fail to new RAMA manager : %v", err)
	}

	if err = mgr.RunOrDie(ctx); err != nil {
		klog.Fatalf("fail to run RAMA manager : %v", err)
	}
}
