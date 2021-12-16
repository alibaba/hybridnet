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

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	daemonconfig "github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/controller"
	"github.com/alibaba/hybridnet/pkg/daemon/server"
	"github.com/alibaba/hybridnet/pkg/feature"
)

var (
	setupLog = ctrl.Log.WithName("setup")

	gitCommit string
)

func main() {
	setupLog.Info("Starting daemon with git commit", gitCommit)
	setupLog.Info("known features: ", feature.KnownFeatures())

	config, err := daemonconfig.ParseFlags()
	if err != nil {
		setupLog.Error(err, "failed to parse config")
		os.Exit(1)
	}

	// setup manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		setupLog.Error(err, "unable to start daemon manager")
		os.Exit(1)
	}

	if err := clientgoscheme.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "failed to add client-go to manager scheme")
		os.Exit(1)
	}

	if err := networkingv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "failed to add networking v1 to manager scheme")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	ctl, err := controller.NewController(config, mgr)
	if err != nil {
		setupLog.Error(err, "failed to create controller")
		os.Exit(1)
	}

	go func() {
		if err = ctl.Run(ctx); err != nil {
			setupLog.Error(err, "controller exit unusually")
			os.Exit(1)
		}
	}()

	server.RunServer(ctx, config, ctl)
}
