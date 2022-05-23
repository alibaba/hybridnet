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

package networking_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/hybridnet/pkg/controllers/networking"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment

	controllerConcurrency map[string]int
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "charts", "hybridnet", "crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: "/Users/hhy/go/src/github.com/hhyasdf/hybridnet/bin",
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = networkingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Logger: ctrl.Log.WithName("manager"),
	})
	Expect(err).ToNot(HaveOccurred())

	// pre-start hooks registration
	var preStartHooks []func() error
	preStartHooks = append(preStartHooks, func() error {
		// TODO: this conversion will be removed in next major version
		return networkingv1.CanonicalizeIPInstance(mgr.GetClient())
	})

	// indexers need to be injected be for informer is running
	Expect(networking.InitIndexers(mgr)).ToNot(HaveOccurred())

	go func() {
		Expect(mgr.Start(context.TODO())).ToNot(HaveOccurred())
	}()

	// wait for manager cache client ready
	mgr.GetCache().WaitForCacheSync(context.TODO())

	// run pre-start hooks
	Expect(errors.AggregateGoroutines(preStartHooks...)).ToNot(HaveOccurred())

	// init IPAM manager and stort
	ipamManager, err := networking.NewIPAMManager(mgr.GetClient())
	if err != nil {
		os.Exit(1)
	}

	podIPCache, err := networking.NewPodIPCache(mgr.GetClient(), ctrl.Log.WithName("pod-ip-cache"))
	Expect(err).ToNot(HaveOccurred())

	ipamStore := networking.NewIPAMStore(mgr.GetClient())

	// setup controllers
	Expect((&networking.IPAMReconciler{
		Client:                mgr.GetClient(),
		Refresh:               ipamManager,
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerIPAM]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	Expect((&networking.IPInstanceReconciler{
		Client:                mgr.GetClient(),
		PodIPCache:            podIPCache,
		IPAMManager:           ipamManager,
		IPAMStore:             ipamStore,
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerIPInstance]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	Expect((&networking.NodeReconciler{
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerNode]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	Expect((&networking.PodReconciler{
		APIReader:             mgr.GetAPIReader(),
		Client:                mgr.GetClient(),
		PodIPCache:            podIPCache,
		IPAMStore:             ipamStore,
		IPAMManager:           ipamManager,
		Recorder:              &record.FakeRecorder{},
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerPod]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	Expect((&networking.NetworkStatusReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		Recorder:              &record.FakeRecorder{},
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerNetworkStatus]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	Expect((&networking.SubnetStatusReconciler{
		Client:                mgr.GetClient(),
		IPAMManager:           ipamManager,
		Recorder:              &record.FakeRecorder{},
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerSubnetStatus]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	Expect((&networking.QuotaReconciler{
		Client:                mgr.GetClient(),
		ControllerConcurrency: concurrency.ControllerConcurrency(controllerConcurrency[networking.ControllerQuota]),
	}).SetupWithManager(mgr)).ToNot(HaveOccurred())

	// An underlay network and an overlay network.
	// Two nodes, only one with underlay network attached.
	ctx := context.Background()
	netID := int32(100)

	underlayNetworkName := "underlay-network"
	overlayNetworkName := "overlay-network"

	networkList := []*networkingv1.Network{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: underlayNetworkName,
			},
			Spec: networkingv1.NetworkSpec{
				NetID: &netID,
				Type:  networkingv1.NetworkTypeUnderlay,
				Mode:  networkingv1.NetworkModeVlan,
				NodeSelector: map[string]string{
					"network": underlayNetworkName,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: overlayNetworkName,
			},
			Spec: networkingv1.NetworkSpec{
				NetID: &netID,
				Type:  networkingv1.NetworkTypeOverlay,
				Mode:  networkingv1.NetworkModeVxlan,
			},
		},
	}

	subnetList := []*networkingv1.Subnet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "underlay-subnet1",
			},
			Spec: networkingv1.SubnetSpec{
				Network: underlayNetworkName,
				Range: networkingv1.AddressRange{
					Version: "4",
					CIDR:    "192.168.56.0/24",
					Gateway: "192.168.56.1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "overlay-subnet1",
			},
			Spec: networkingv1.SubnetSpec{
				Network: overlayNetworkName,
				Range: networkingv1.AddressRange{
					Version: "4",
					CIDR:    "100.10.0.0/16",
				},
			},
		},
	}

	nodeList := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node1",
				Labels: map[string]string{
					"network": "underlay-network",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node2",
				Labels: map[string]string{
					"network": "underlay-network",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node3",
			},
		},
	}

	for _, network := range networkList {
		Expect(k8sClient.Create(ctx, network)).Should(Succeed())
	}
	for _, subnet := range subnetList {
		Expect(k8sClient.Create(ctx, subnet)).Should(Succeed())
	}
	for _, node := range nodeList {
		Expect(k8sClient.Create(ctx, node)).Should(Succeed())
	}

}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
