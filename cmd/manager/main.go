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
	"flag"
	"fmt"
	"os"

	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/multicluster"
	"github.com/alibaba/hybridnet/pkg/controllers/networking"
	"github.com/alibaba/hybridnet/pkg/feature"
	zapinit "github.com/alibaba/hybridnet/pkg/zap"
)

var (
	gitCommit string
	scheme    = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(multiclusterv1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(kubevirtv1.AddToScheme(scheme))
}

func main() {
	var (
		controllerConcurrency map[string]int
		clientQPS             float32
		clientBurst           int
		metricsPort           int
	)

	// register flags
	pflag.StringToIntVar(&controllerConcurrency, "controller-concurrency", map[string]int{}, "The specified concurrency of different controllers.")
	pflag.Float32Var(&clientQPS, "kube-client-qps", 300, "The QPS limit of apiserver client.")
	pflag.IntVar(&clientBurst, "kube-client-burst", 600, "The Burst limit of apiserver client.")
	pflag.IntVar(&metricsPort, "metrics-port", 9899, "The port to listen on for prometheus metrics.")

	// parse flags
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrllog.SetLogger(zapinit.NewZapLogger())

	var entryLog = ctrllog.Log.WithName("entry")
	entryLog.Info("starting hybridnet manager",
		"known-features", feature.KnownFeatures(),
		"commit-id", gitCommit,
		"controller-concurrency", controllerConcurrency)

	globalContext := ctrl.SetupSignalHandler()

	clientConfig := ctrl.GetConfigOrDie()
	clientConfig.QPS = clientQPS
	clientConfig.Burst = clientBurst

	mgr, err := ctrl.NewManager(clientConfig, ctrl.Options{
		Scheme:                  scheme,
		Logger:                  ctrl.Log.WithName("manager"),
		MetricsBindAddress:      fmt.Sprintf(":%d", metricsPort),
		LeaderElection:          true,
		LeaderElectionID:        "hybridnet-manager-election",
		LeaderElectionNamespace: os.Getenv("NAMESPACE"),
	})
	if err != nil {
		entryLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// indexers need to be injected be for informer is running
	if err = networking.InitIndexers(mgr); err != nil {
		entryLog.Error(err, "unable to init indexers")
		os.Exit(1)
	}

	go func() {
		if err := mgr.Start(globalContext); err != nil {
			entryLog.Error(err, "manager exit unexpectedly")
			os.Exit(1)
		}
	}()

	// Initialization should be after leader election success
	<-mgr.Elected()

	// wait for manager cache client ready
	mgr.GetCache().WaitForCacheSync(globalContext)

	if err = networking.RegisterToManager(globalContext, mgr, networking.RegisterOptions{
		ConcurrencyMap: controllerConcurrency,
	}); err != nil {
		entryLog.Error(err, "unable to register networking controllers")
		os.Exit(1)
	}

	if feature.MultiClusterEnabled() {
		if err = multicluster.RegisterToManager(globalContext, mgr, multicluster.RegisterOptions{
			ConcurrencyMap: controllerConcurrency,
		}); err != nil {
			entryLog.Error(err, "unable to register multi-cluster controllers")
			os.Exit(1)
		}
	}

	<-globalContext.Done()
}
