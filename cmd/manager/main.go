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
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	globalutils "github.com/alibaba/hybridnet/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/multicluster"
	"github.com/alibaba/hybridnet/pkg/controllers/multicluster/clusterchecker"
	"github.com/alibaba/hybridnet/pkg/controllers/networking"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/managerruntime"
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

	signalContext := ctrl.SetupSignalHandler()

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
	if err = initIndexers(mgr); err != nil {
		entryLog.Error(err, "unable to init indexers")
		os.Exit(1)
	}

	go func() {
		if err := mgr.Start(signalContext); err != nil {
			entryLog.Error(err, "manager exit unexpectedly")
			os.Exit(1)
		}
	}()

	mgr.GetCache().WaitForCacheSync(signalContext)
	ipamManager, err := networking.NewIPAMManager(mgr.GetClient())
	if err != nil {
		entryLog.Error(err, "unable to create IPAM manager")
		os.Exit(1)
	}

	ipamStore := networking.NewIPAMStore(mgr.GetClient())

	if err = (&networking.IPAMReconciler{
		Client:                mgr.GetClient(),
		Refresh:               ipamManager,
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerIPAM]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerIPAM)
		os.Exit(1)
	}

	if err = (&networking.IPInstanceReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		IPAMStore:             ipamStore,
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerIPInstance]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerIPInstance)
		os.Exit(1)
	}

	if err = (&networking.NodeReconciler{
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerNode]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerNode)
		os.Exit(1)
	}

	if err = (&networking.PodReconciler{
		APIReader:             mgr.GetAPIReader(),
		Client:                mgr.GetClient(),
		Recorder:              mgr.GetEventRecorderFor(networking.ControllerPod + "Controller"),
		IPAMStore:             ipamStore,
		IPAMManager:           ipamManager,
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerPod]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerPod)
		os.Exit(1)
	}

	if err = (&networking.NetworkStatusReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		Recorder:              mgr.GetEventRecorderFor(networking.ControllerNetworkStatus + "Controller"),
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerNetworkStatus]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerNetworkStatus)
		os.Exit(1)
	}

	if err = (&networking.SubnetStatusReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		Recorder:              mgr.GetEventRecorderFor(networking.ControllerSubnetStatus + "Controller"),
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerSubnetStatus]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerSubnetStatus)
		os.Exit(1)
	}

	if err = (&networking.QuotaReconciler{
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerQuota]),
	}).SetupWithManager(mgr); err != nil {
		entryLog.Error(err, "unable to inject controller", "controller", networking.ControllerQuota)
		os.Exit(1)
	}

	if feature.MultiClusterEnabled() {
		clusterCheckEvent := make(chan multicluster.ClusterCheckEvent, 5)

		uuidMutex, err := multicluster.NewUUIDMutexFromClient(mgr.GetClient())
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
			Client:                mgr.GetClient(),
			Recorder:              mgr.GetEventRecorderFor(multicluster.ControllerRemoteClusterUUID + "Controller"),
			UUIDMutex:             uuidMutex,
			ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[multicluster.ControllerRemoteClusterUUID]),
		}).SetupWithManager(mgr); err != nil {
			entryLog.Error(err, "unable to inject controller", "controller", multicluster.ControllerRemoteClusterUUID)
			os.Exit(1)
		}

		if err = (&multicluster.RemoteClusterReconciler{
			Client:                mgr.GetClient(),
			Recorder:              mgr.GetEventRecorderFor(multicluster.ControllerRemoteCluster + "Controller"),
			UUIDMutex:             uuidMutex,
			DaemonHub:             daemonHub,
			LocalManager:          mgr,
			Event:                 clusterCheckEvent,
			ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[multicluster.ControllerRemoteCluster]),
		}).SetupWithManager(mgr); err != nil {
			entryLog.Error(err, "unable to inject controller", "controller", multicluster.ControllerRemoteCluster)
			os.Exit(1)
		}

		if err = mgr.Add(&multicluster.RemoteClusterStatusChecker{
			Client:      mgr.GetClient(),
			Logger:      mgr.GetLogger().WithName("checker").WithName(multicluster.CheckerRemoteClusterStatus),
			CheckPeriod: 30 * time.Second,
			DaemonHub:   daemonHub,
			Checker:     clusterStatusChecker,
			Event:       clusterCheckEvent,
			Recorder:    mgr.GetEventRecorderFor(multicluster.CheckerRemoteClusterStatus + "Checker"),
		}); err != nil {
			entryLog.Error(err, "unable to inject checker", "checker", multicluster.CheckerRemoteClusterStatus)
			os.Exit(1)
		}
	}

	<-signalContext.Done()
}

func initClusterStatusChecker(mgr ctrl.Manager) (clusterchecker.Checker, error) {
	clusterUUID, err := utils.GetClusterUUID(mgr.GetClient())
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
	if err = checker.Register(clusterchecker.SubnetCheckName, &clusterchecker.Subnet{LocalClient: mgr.GetClient()}); err != nil {
		return nil, err
	}
	return checker, nil
}

func initIndexers(mgr ctrl.Manager) (err error) {
	// init node indexer for networks
	if err = mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.Network{},
		networking.IndexerFieldNode, func(obj client.Object) []string {
			network, ok := obj.(*networkingv1.Network)
			if !ok {
				return nil
			}

			switch networkingv1.GetNetworkType(network) {
			case networkingv1.NetworkTypeUnderlay:
				return globalutils.DeepCopyStringSlice(network.Status.NodeList)
			case networkingv1.NetworkTypeOverlay:
				return []string{networking.OverlayNodeName}
			case networkingv1.NetworkTypeGlobalBGP:
				return []string{networking.GlobalBGPNodeName}
			default:
				return nil
			}
		}); err != nil {
		return err
	}

	// init network indexer for Subnets
	return mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.Subnet{},
		networking.IndexerFieldNetwork, func(obj client.Object) []string {
			subnet, ok := obj.(*networkingv1.Subnet)
			if !ok {
				return nil
			}

			networkName := subnet.Spec.Network
			if len(networkName) > 0 {
				return []string{networkName}
			}
			return nil
		})
}
