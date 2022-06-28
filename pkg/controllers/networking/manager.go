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

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
)

func RegisterToManager(ctx context.Context, mgr manager.Manager, concurrencyMap map[string]int) error {
	// init IPAM manager and start
	ipamManager, err := NewIPAMManager(ctx, mgr.GetClient())
	if err != nil {
		return fmt.Errorf("unable to create IPAM manager: %v", err)
	}

	podIPCache, err := NewPodIPCache(ctx, mgr.GetClient(), ctrllog.Log.WithName("pod-ip-cache"))
	if err != nil {
		return fmt.Errorf("unable to create Pod IP cache: %v", err)
	}

	ipamStore := NewIPAMStore(mgr.GetClient())

	// setup controllers
	if err = (&IPAMReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerIPAM]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerIPAM, err)
	}

	if err = (&IPInstanceReconciler{
		Client:                mgr.GetClient(),
		PodIPCache:            podIPCache,
		IPAMManager:           ipamManager,
		IPAMStore:             ipamStore,
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerIPInstance]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerIPInstance, err)
	}

	if err = (&NodeReconciler{
		Context:               ctx,
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerNode]),
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
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerPod]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerPod, err)
	}

	if err = (&NetworkStatusReconciler{
		Context:               ctx,
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		Recorder:              mgr.GetEventRecorderFor(ControllerNetworkStatus + "Controller"),
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerNetworkStatus]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerNetworkStatus, err)
	}

	if err = (&SubnetStatusReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		Recorder:              mgr.GetEventRecorderFor(ControllerSubnetStatus + "Controller"),
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerSubnetStatus]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerSubnetStatus, err)
	}

	if err = (&QuotaReconciler{
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerQuota]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerQuota, err)
	}

	return nil
}
