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
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/networking"
	"github.com/alibaba/hybridnet/pkg/utils"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient   client.Client
	testEnv     *envtest.Environment
	ipamManager networking.IPAMManager
)

var testLock = sync.Mutex{}

const (
	underlayNetworkName   = "underlay-network"
	overlayNetworkName    = "overlay-network"
	underlaySubnetName    = "underlay-subnet"
	overlayIPv4SubnetName = "overlay-ipv4-subnet"
	overlayIPv6SubnetName = "overlay-ipv6-subnet"
	node1Name             = "node1"
	node2Name             = "node2"
	node3Name             = "node3"
	netID                 = 100
)

const (
	basicIPQuantity  int32 = 256
	networkAddress   int32 = 1
	gatewayAddress   int32 = 1
	broadcastAddress int32 = 1
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Networking Controllers Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "charts", "hybridnet", "crds")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	Expect(scheme).NotTo(BeNil())
	Expect(clientgoscheme.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(networkingv1.AddToScheme(scheme)).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		Logger:                  ctrl.Log.WithName("manager"),
		LeaderElection:          true,
		LeaderElectionID:        "hybridnet-manager-election",
		LeaderElectionNamespace: "kube-system",
	})
	Expect(err).NotTo(HaveOccurred())

	// indexers need to be injected be for informer is running
	Expect(networking.InitIndexers(mgr)).ToNot(HaveOccurred())

	go func() {
		Expect(mgr.Start(context.TODO())).ToNot(HaveOccurred())
	}()

	// wait for manager cache client ready
	Eventually(mgr.GetCache().WaitForCacheSync(context.TODO())).
		WithTimeout(time.Minute).
		WithPolling(3 * time.Second).
		Should(BeTrue())

	// register all controllers to manager
	Expect(networking.RegisterToManager(context.TODO(), mgr, networking.RegisterOptions{
		// use custom IPAM manager constructor to fetch IPAM manager
		NewIPAMManager: func(ctx context.Context, c client.Client) (networking.IPAMManager, error) {
			ipamManager, err = networking.NewIPAMManager(ctx, c)
			return ipamManager, err
		},
	})).NotTo(HaveOccurred())

	// An underlay network and an overlay network.
	// Three nodes, two with underlay network attached.
	ctx := context.Background()

	networkList := []*networkingv1.Network{
		underlayNetworkRender(underlayNetworkName, netID),
		overlayNetworkRender(overlayNetworkName, netID),
	}

	subnetList := []*networkingv1.Subnet{
		subnetRender(underlaySubnetName, underlayNetworkName, "192.168.56.0/24", nil, true),
		subnetRender(overlayIPv4SubnetName, overlayNetworkName, "100.10.0.0/24", pointer.Int32Ptr(44), false),
		subnetRender(overlayIPv6SubnetName, overlayNetworkName, "fe80::0/120", pointer.Int32Ptr(66), false),
	}

	nodeList := []*corev1.Node{
		nodeRender(node1Name, map[string]string{
			"network": underlayNetworkName,
		}),
		nodeRender(node2Name, map[string]string{
			"network": underlayNetworkName,
		}),
		nodeRender(node3Name, map[string]string{}),
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

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	Expect(testEnv.Stop()).NotTo(HaveOccurred())
})

// underlayNetworkRender will render a simple underlay network of vlan mode
func underlayNetworkRender(name string, netID int32) *networkingv1.Network {
	return &networkingv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1.NetworkSpec{
			NodeSelector: map[string]string{
				"network": name,
			},
			NetID: &netID,
			Type:  networkingv1.NetworkTypeUnderlay,
			Mode:  networkingv1.NetworkModeVlan,
		},
	}
}

// overlayNetworkRender will render a simple overlay network of vxlan mode
func overlayNetworkRender(name string, netID int32) *networkingv1.Network {
	return &networkingv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1.NetworkSpec{
			NetID: &netID,
			Type:  networkingv1.NetworkTypeOverlay,
			Mode:  networkingv1.NetworkModeVxlan,
		},
	}
}

// subnetRender will render a simple subnet of a specified network
func subnetRender(name, networkName string, cidr string, netID *int32, hasGateway bool) *networkingv1.Subnet {
	tempIP, _, _ := net.ParseCIDR(cidr)
	return &networkingv1.Subnet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkingv1.SubnetSpec{
			Range: networkingv1.AddressRange{
				Version: func() networkingv1.IPVersion {
					if tempIP.To4() == nil {
						return networkingv1.IPv6
					}
					return networkingv1.IPv4
				}(),
				CIDR: cidr,
				Gateway: func() string {
					if hasGateway {
						return utils.NextIP(tempIP).String()
					}
					return ""
				}(),
			},
			NetID:   netID,
			Network: networkName,
		},
	}
}

// nodeRender will render a simple node with assigned labels
func nodeRender(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}
