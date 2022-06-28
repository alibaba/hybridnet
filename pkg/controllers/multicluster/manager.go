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

package multicluster

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/managerruntime"
)

func RegisterToManager(ctx context.Context, mgr manager.Manager, concurrencyMap map[string]int) error {
	clusterCheckEvent := make(chan ClusterCheckEvent, 5)

	uuidMutex, err := NewUUIDMutexFromClient(ctx, mgr.GetClient())
	if err != nil {
		return fmt.Errorf("unable to create cluster UUID mutex: %v", err)
	}

	daemonHub := managerruntime.NewDaemonHub(ctx)

	clusterStatusChecker, err := InitClusterStatusChecker(ctx, mgr)
	if err != nil {
		return fmt.Errorf("unable to init cluster status checker: %v", err)
	}

	if err = (&RemoteClusterUUIDReconciler{
		Client:                mgr.GetClient(),
		Recorder:              mgr.GetEventRecorderFor(ControllerRemoteClusterUUID + "Controller"),
		UUIDMutex:             uuidMutex,
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerRemoteClusterUUID]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerRemoteClusterUUID, err)
	}

	if err = (&RemoteClusterReconciler{
		Context:               ctx,
		Client:                mgr.GetClient(),
		Recorder:              mgr.GetEventRecorderFor(ControllerRemoteCluster + "Controller"),
		UUIDMutex:             uuidMutex,
		DaemonHub:             daemonHub,
		LocalManager:          mgr,
		Event:                 clusterCheckEvent,
		ControllerConcurrency: concurrency.ControllerConcurrency(concurrencyMap[ControllerRemoteCluster]),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to inject controller %s: %v", ControllerRemoteCluster, err)
	}

	if err = mgr.Add(&RemoteClusterStatusChecker{
		Client:      mgr.GetClient(),
		Logger:      mgr.GetLogger().WithName("checker").WithName(CheckerRemoteClusterStatus),
		CheckPeriod: 30 * time.Second,
		DaemonHub:   daemonHub,
		Checker:     clusterStatusChecker,
		Event:       clusterCheckEvent,
		Recorder:    mgr.GetEventRecorderFor(CheckerRemoteClusterStatus + "Checker"),
	}); err != nil {
		return fmt.Errorf("unable to inject checker %s: %v", CheckerRemoteClusterStatus, err)
	}

	return nil
}
