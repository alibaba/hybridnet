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
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	daemonconfig "github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/controller"
	"github.com/alibaba/hybridnet/pkg/daemon/server"
	"github.com/alibaba/hybridnet/pkg/feature"
)

var gitCommit string

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()

	klog.Infof("Starting hybridnet daemon with git commit: %v", gitCommit)
	klog.Infof("known features: %v", feature.KnownFeatures())

	config, err := daemonconfig.ParseFlags()
	if err != nil {
		klog.Fatalf("failed to parse config: %v", err)
	}

	// setup manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		klog.Fatalf("unable to start daemon manager %v", err)
	}

	if err := clientgoscheme.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("failed to add client-go to manager scheme %v", err)
	}

	if err := networkingv1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("failed to add networking v1 to manager scheme %v", err)
	}

	ctx := ctrl.SetupSignalHandler()

	ctl, err := controller.NewController(config, mgr)
	if err != nil {
		klog.Fatalf("failed to create controller %v", err)
	}

	mgr.GetAPIReader()

	go func() {
		if err = ctl.Run(ctx); err != nil {
			klog.Fatalf("controller exit unusually %v", err)
		}
	}()

	server.RunServer(ctx, config, ctl)
}
