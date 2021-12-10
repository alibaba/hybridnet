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
	hybridnetinformer "github.com/alibaba/hybridnet/pkg/client/informers/externalversions"
	daemonconfig "github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/controller"
	"github.com/alibaba/hybridnet/pkg/daemon/server"
	"github.com/alibaba/hybridnet/pkg/feature"

	"k8s.io/client-go/informers"
	"k8s.io/klog"
)

var gitCommit string

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()

	klog.Infof("Starting hybridnet daemon with git commit: %v", gitCommit)
	klog.Infof("known features: %v", feature.KnownFeatures())

	config, err := daemonconfig.ParseFlags()
	if err != nil {
		klog.Fatalf("parse config failed: %v", err)
	}

	// TODO: remove the stop channel
	stopCh := make(chan struct{})
	hybridnetInformerFactory := hybridnetinformer.NewSharedInformerFactory(config.HybridnetClient, 0)
	kubeInformerFactory := informers.NewSharedInformerFactory(config.KubeClient, 0)

	ctl, err := controller.NewController(config, hybridnetInformerFactory, kubeInformerFactory)
	if err != nil {
		klog.Fatalf("create controller failed %v", err)
	}

	go hybridnetInformerFactory.Start(stopCh)
	go kubeInformerFactory.Start(stopCh)

	go func() {
		if err = ctl.Run(stopCh); err != nil {
			klog.Fatalf("controller exit unusually %v", err)
		}
	}()

	server.RunServer(stopCh, config, ctl)
}
