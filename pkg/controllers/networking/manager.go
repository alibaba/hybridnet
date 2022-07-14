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

package networking

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
)

type RegisterOptions struct {
	NewIPAMManager NewIPAMManagerFunction
	ConcurrencyMap map[string]int
}

func RegisterToManager(ctx context.Context, mgr manager.Manager, options RegisterOptions) error {
	if options.NewIPAMManager == nil {
		options.NewIPAMManager = NewIPAMManager
	}
	if len(options.ConcurrencyMap) == 0 {
		options.ConcurrencyMap = map[string]int{}
	}

	// init IPAM manager and start
	ipamManager, err := options.NewIPAMManager(ctx, mgr.GetClient())
	if err != nil {
		return fmt.Errorf("unable to create IPAM manager: %v", err)
	}

	podIPCache, err := NewPodIPCache(ctx, mgr.GetClient(), ctrllog.Log.WithName("pod-ip-cache"))
	if err != nil {
		return fmt.Errorf("unable to create Pod IP cache: %v", err)
	}

	ipamStore := NewIPAMStore(mgr.GetClient())

	// init status update channels
	networkStatusUpdateChan, subnetStatusUpdateChan := make(chan event.GenericEvent), make(chan event.GenericEvent)

	// setup controllers
	if err = (&IPAMReconciler{
		Client:                  mgr.GetClient(),
		IPAMManager:             ipamManager,
		NetworkStatusUpdateChan: networkStatusUpdateChan,
		SubnetStatusUpdateChan:  subnetStatusUpdateChan,
		ControllerConcurrency:   concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerIPAM]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerIPAM, err)
	}

	if err = (&IPInstanceReconciler{
		Client:                mgr.GetClient(),
		PodIPCache:            podIPCache,
		IPAMManager:           ipamManager,
		IPAMStore:             ipamStore,
		ControllerConcurrency: concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerIPInstance]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerIPInstance, err)
	}

	if err = (&NodeReconciler{
		Context:               ctx,
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerNode]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerNode, err)
	}

	if err = (&PodReconciler{
		APIReader:             mgr.GetAPIReader(),
		Client:                mgr.GetClient(),
		Recorder:              mgr.GetEventRecorderFor(ControllerPod + "Controller"),
		PodIPCache:            podIPCache,
		IPAMStore:             ipamStore,
		IPAMManager:           ipamManager,
		ControllerConcurrency: concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerPod]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerPod, err)
	}

	if err = (&NetworkStatusReconciler{
		Context:                 ctx,
		Client:                  mgr.GetClient(),
		IPAMManager:             ipamManager,
		Recorder:                mgr.GetEventRecorderFor(ControllerNetworkStatus + "Controller"),
		NetworkStatusUpdateChan: networkStatusUpdateChan,
		ControllerConcurrency:   concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerNetworkStatus]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerNetworkStatus, err)
	}

	if err = (&SubnetStatusReconciler{
		Client:                 mgr.GetClient(),
		IPAMManager:            ipamManager,
		Recorder:               mgr.GetEventRecorderFor(ControllerSubnetStatus + "Controller"),
		SubnetStatusUpdateChan: subnetStatusUpdateChan,
		ControllerConcurrency:  concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerSubnetStatus]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerSubnetStatus, err)
	}

	if err = (&QuotaReconciler{
		Context:               ctx,
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(options.ConcurrencyMap[ControllerQuota]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerQuota, err)
	}

	return nil
}
