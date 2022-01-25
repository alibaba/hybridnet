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
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/multicluster"
	"github.com/alibaba/hybridnet/pkg/controllers/multicluster/clusterchecker"
	"github.com/alibaba/hybridnet/pkg/controllers/networking"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/managerruntime"
)

var (
	gitCommit string
	scheme    = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(multiclusterv1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
}

func main() {
	// parse flags
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrllog.SetLogger(zap.New(zap.UseDevMode(true)))

	var entryLog = ctrllog.Log.WithName("entry")
	entryLog.Info("starting hybridnet manager", "known-features", feature.KnownFeatures(), "commit-id", gitCommit)

	signalContext := ctrl.SetupSignalHandler()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Logger: ctrl.Log.WithName("manager"),
	})
	if err != nil {
		entryLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ipamManager, err := networking.NewIPAMManager(mgr.GetAPIReader())
	if err != nil {
		entryLog.Error(err, "unable to create IPAM manager")
		os.Exit(1)
	}

	ipamStore := networking.NewIPAMStore(mgr.GetClient())

	if err = (&networking.IPAMReconciler{
		Client:  mgr.GetClient(),
		Refresh: ipamManager,
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "IPAM")
		os.Exit(1)
	}

	if err = (&networking.IPInstanceReconciler{
		Client:      mgr.GetClient(),
		IPAMManager: ipamManager,
		IPAMStore:   ipamStore,
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "IPInstance")
		os.Exit(1)
	}

	if err = (&networking.NodeReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "Node")
		os.Exit(1)
	}

	if err = (&networking.PodReconciler{
		APIReader:   mgr.GetAPIReader(),
		Client:      mgr.GetClient(),
		Recorder:    mgr.GetEventRecorderFor("PodController"),
		IPAMStore:   ipamStore,
		IPAMManager: ipamManager,
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "Pod")
		os.Exit(1)
	}

	if err = (&networking.NetworkStatusReconciler{
		Client:      mgr.GetClient(),
		IPAMManager: ipamManager,
		Recorder:    mgr.GetEventRecorderFor("NetworkStatusController"),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "NetworkStatus")
		os.Exit(1)
	}

	if err = (&networking.SubnetStatusReconciler{
		Client:      mgr.GetClient(),
		IPAMManager: ipamManager,
		Recorder:    mgr.GetEventRecorderFor("SubnetStatusController"),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "SubnetStatus")
		os.Exit(1)
	}

	if err = (&networking.QuotaReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", "QuotaController")
		os.Exit(1)
	}

	if feature.MultiClusterEnabled() {
		clusterCheckEvent := make(chan multicluster.ClusterCheckEvent, 5)

		uuidMutex, err := multicluster.NewUUIDMutexFromClient(mgr.GetAPIReader())
		if err != nil {
			entryLog.Error(err, "unable to create cluster UUID mutex")
			os.Exit(1)
		}

		daemonHub := managerruntime.NewDaemonHub(signalContext)

		clusterStatusChecker, err := initClusterStatusChecker(mgr)
		if err != nil {
			entryLog.Error(err, "unable to init cluster status checker")
			os.Exit(1)
		}

		if err = (&multicluster.RemoteClusterUUIDReconciler{
			Client:    mgr.GetClient(),
			Recorder:  mgr.GetEventRecorderFor("RemoteClusterUUIDController"),
			UUIDMutex: uuidMutex,
		}).SetupWithManager(mgr); err != nil {
			entryLog.Error(err, "unable to inject controller", "controller", "RemoteClusterUUID")
			os.Exit(1)
		}

		if err = (&multicluster.RemoteClusterReconciler{
			Client:       mgr.GetClient(),
			Recorder:     mgr.GetEventRecorderFor("RemoteClusterController"),
			UUIDMutex:    uuidMutex,
			DaemonHub:    daemonHub,
			LocalManager: mgr,
			Event:        clusterCheckEvent,
		}).SetupWithManager(mgr); err != nil {
			entryLog.Error(err, "unable to inject controller", "controller", "RemoteCluster")
			os.Exit(1)
		}

		if err = mgr.Add(&multicluster.RemoteClusterStatusChecker{
			Client:      mgr.GetClient(),
			Logger:      mgr.GetLogger().WithName("RemoteClusterStatusChecker"),
			CheckPeriod: 2 * time.Minute,
			DaemonHub:   daemonHub,
			Checker:     clusterStatusChecker,
			Event:       clusterCheckEvent,
			Recorder:    mgr.GetEventRecorderFor("RemoteClusterStatusChecker"),
		}); err != nil {
			entryLog.Error(err, "unable to inject checker", "checker", "RemoteClusterStatus")
			os.Exit(1)
		}
	}

	// TODO: migrate to manager
	go startMetricsServer()

	if err = mgr.Start(signalContext); err != nil {
		entryLog.Error(err, "manager exit unexpectedly")
		os.Exit(1)
	}
}

func initClusterStatusChecker(mgr ctrl.Manager) (clusterchecker.Checker, error) {
	clusterUUID, err := utils.GetClusterUUID(mgr.GetAPIReader())
	if err != nil {
		return nil, fmt.Errorf("unable to get cluster UUID: %v", err)
	}

	checker := clusterchecker.NewChecker()

	if err = checker.Register(clusterchecker.HealthzCheckName, &clusterchecker.Healthz{}); err != nil {
		return nil, err
	}
	if err = checker.Register(clusterchecker.BidirectionCheckName, &clusterchecker.Bidirection{LocalUUID: clusterUUID}); err != nil {
		return nil, err
	}
	if err = checker.Register(clusterchecker.OverlayNetIDCheckName, &clusterchecker.OverlayNetID{LocalClient: mgr.GetClient()}); err != nil {
		return nil, err
	}
	return checker, nil
}
