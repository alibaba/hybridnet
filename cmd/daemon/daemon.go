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
	_ "net/http/pprof"

	ramainformer "github.com/oecp/rama/pkg/client/informers/externalversions"
	daemonconfig "github.com/oecp/rama/pkg/daemon/config"
	"github.com/oecp/rama/pkg/daemon/controller"
	"github.com/oecp/rama/pkg/daemon/server"
	"k8s.io/client-go/informers"
	"k8s.io/klog"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

var gitCommit string

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()

	klog.Infof("Starting rama daemon with git commit: %v", gitCommit)

	config, err := daemonconfig.ParseFlags()
	if err != nil {
		klog.Fatalf("parse config failed: %v", err)
	}

	stopCh := controllerruntime.SetupSignalHandler()
	ramaInformerFactory := ramainformer.NewSharedInformerFactory(config.RamaClient, 0)
	kubeInformerFactory := informers.NewSharedInformerFactory(config.KubeClient, 0)

	ctl, err := controller.NewController(config, ramaInformerFactory, kubeInformerFactory)
	if err != nil {
		klog.Fatalf("create controller failed %v", err)
	}

	go ramaInformerFactory.Start(stopCh)
	go kubeInformerFactory.Start(stopCh)

	go func() {
		if err = ctl.Run(stopCh); err != nil {
			klog.Fatalf("controller exit unusually %v", err)
		}
	}()

	server.RunServer(stopCh, config, ctl)
}
