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
	"os"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/alibaba/hybridnet/pkg/feature"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	daemonconfig "github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/controller"
	"github.com/alibaba/hybridnet/pkg/daemon/server"
)

var gitCommit string

func main() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	var entryLog = log.Log.WithName("entry")
	entryLog.Info("starting hybridnet daemon",
		"known-features", feature.KnownFeatures(), "commit-id", gitCommit)

	config, err := daemonconfig.ParseFlags()
	if err != nil {
		entryLog.Error(err, "failed to parse config")
		os.Exit(1)
	}
	entryLog.Info("generate daemon config", "config", *config)

	// setup manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		entryLog.Error(err, "unable to start daemon manager")
		os.Exit(1)
	}

	if err := clientgoscheme.AddToScheme(mgr.GetScheme()); err != nil {
		entryLog.Error(err, "failed to add client-go to manager scheme")
		os.Exit(1)
	}

	if err := networkingv1.AddToScheme(mgr.GetScheme()); err != nil {
		entryLog.Error(err, "failed to add networking v1 to manager scheme")
		os.Exit(1)
	}

	if err := multiclusterv1.AddToScheme(mgr.GetScheme()); err != nil {
		entryLog.Error(err, "failed to add multicluster v1 to manager scheme")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	ctl, err := controller.NewCtrlHub(config, mgr, log.Log.WithName("ctrl-hub"))
	if err != nil {
		entryLog.Error(err, "failed to create controller")
		os.Exit(1)
	}

	mgr.GetAPIReader()

	go func() {
		if err = ctl.Run(ctx); err != nil {
			entryLog.Error(err, "CtrlHub exit unusually")
			os.Exit(1)
		}
	}()

	server.RunServer(ctx, config, ctl, log.Log.WithName("cni-server"))
}
