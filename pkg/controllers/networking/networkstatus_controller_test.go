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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("Network status test", func() {
	It("Node list update", func() {
		By("Expecting to have two node fo underlay network status")
		Eventually(func() error {
			network := &networkingv1.Network{}
			if err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name: "underlay-network",
			}, network); err != nil {
				return err
			}

			if len(network.Status.NodeList) != 2 {
				return fmt.Errorf("error number of network node list, supposed to be 2, but is %v",
					len(network.Status.NodeList))
			}

			return nil
		}, timeout, interval).Should(Succeed())

		By("Expecting update node list of network status after create new nodes")
		ctx := context.Background()
		testNodeList := []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tmp-node1",
					Labels: map[string]string{
						"network": "underlay-network",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tmp-node2",
					Labels: map[string]string{
						"network": "underlay-network",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tmp-node3",
					Labels: map[string]string{
						"network": "underlay-network",
					},
				},
			},
		}

		for _, node := range testNodeList {
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		}

		Eventually(func() error {
			network := &networkingv1.Network{}
			if err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name: "underlay-network",
			}, network); err != nil {
				return err
			}

			// the target number should not be less than actual node number
			if len(network.Status.NodeList) != 5 {
				return fmt.Errorf("error number of network node list, supposed to be 4, but is %v",
					len(network.Status.NodeList))
			}

			return nil
		}, timeout, interval).Should(Succeed())

		for _, node := range testNodeList {
			Expect(k8sClient.Delete(ctx, node)).Should(Succeed())
		}
	})

	It("Subnet list and statistics update", func() {
		By("Expecting to have two node fo underlay network status")
		Eventually(func() error {
			network := &networkingv1.Network{}
			if err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name: "underlay-network",
			}, network); err != nil {
				return err
			}

			if len(network.Status.SubnetList) != 1 {
				return fmt.Errorf("error number of network node list, supposed to be 2, but is %v",
					len(network.Status.SubnetList))
			}

			return nil
		}, timeout, interval).Should(Succeed())

		By("Expecting update node list of network status after create new subnets")
		ctx := context.Background()
		testSubnetList := []*networkingv1.Subnet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tmp-subnet1",
				},
				Spec: networkingv1.SubnetSpec{
					Network: "underlay-network",
					Range: networkingv1.AddressRange{
						Version: "4",
						CIDR:    "192.168.57.0/24",
						Gateway: "192.168.57.1",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tmp-subnet2",
				},
				Spec: networkingv1.SubnetSpec{
					Network: "underlay-network",
					Range: networkingv1.AddressRange{
						Version: "4",
						CIDR:    "192.168.58.0/24",
						Gateway: "192.168.58.1",
					},
				},
			},
		}

		for _, subnet := range testSubnetList {
			Expect(k8sClient.Create(ctx, subnet)).Should(Succeed())
		}

		Eventually(func() error {
			network := &networkingv1.Network{}
			if err := k8sClient.Get(context.TODO(), types.NamespacedName{
				Name: "underlay-network",
			}, network); err != nil {
				return err
			}

			// the target number should not be less than actual node number
			if len(network.Status.SubnetList) != 3 {
				return fmt.Errorf("error number of network node list, supposed to be 4, but is %v",
					len(network.Status.NodeList))
			}

			if network.Status.Statistics.Total != 253*3 {
				return fmt.Errorf("error number of network statistics total ip, supposed to be %v, but is %v", 253*3,
					network.Status.Statistics.Total)
			}

			// TODO: test updating of "used" and "available" statistics

			return nil
		}, timeout, interval).Should(Succeed())

		for _, subnet := range testSubnetList {
			Expect(k8sClient.Delete(ctx, subnet)).Should(Succeed())
		}
	})
})
