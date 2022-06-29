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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	//+kubebuilder:scaffold:imports
)

const underlayNetworkNameForStatus = "underlay-network-for-status"

var _ = Describe("Network status controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("Initialization check", func() {
		It("Check initialized network status", func() {
			By("check initialized underlay network status")
			Eventually(
				func(g Gomega) {
					network, err := utils.GetNetwork(context.Background(), k8sClient, underlayNetworkName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(network).NotTo(BeNil())

					g.Expect(network.Status.NodeList).To(HaveLen(2))
					g.Expect(network.Status.NodeList).To(ConsistOf(node1Name, node2Name))

					g.Expect(network.Status.SubnetList).To(HaveLen(1))
					g.Expect(network.Status.SubnetList).To(ConsistOf(underlaySubnetName))

					g.Expect(network.Status.Statistics).NotTo(BeNil())
					g.Expect(network.Status.Statistics.Total).Should(Equal(int32(253)))
					g.Expect(network.Status.Statistics.Available).Should(Equal(int32(253)))
					g.Expect(network.Status.Statistics.Used).Should(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check initialized overlay network status")
			Eventually(
				func(g Gomega) {
					network, err := utils.GetNetwork(context.Background(), k8sClient, overlayNetworkName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(network).NotTo(BeNil())

					g.Expect(network.Status.NodeList).To(HaveLen(3))
					g.Expect(network.Status.NodeList).To(ConsistOf(node1Name, node2Name, node3Name))

					g.Expect(network.Status.Statistics).NotTo(BeNil())
					g.Expect(network.Status.Statistics.Total).Should(Equal(int32(254)))
					g.Expect(network.Status.Statistics.Available).Should(Equal(int32(254)))
					g.Expect(network.Status.Statistics.Used).Should(Equal(int32(0)))

					g.Expect(network.Status.IPv6Statistics).NotTo(BeNil())
					g.Expect(network.Status.IPv6Statistics.Total).Should(Equal(int32(255)))
					g.Expect(network.Status.IPv6Statistics.Available).Should(Equal(int32(255)))
					g.Expect(network.Status.IPv6Statistics.Used).Should(Equal(int32(0)))

					g.Expect(network.Status.DualStackStatistics).NotTo(BeNil())
					g.Expect(network.Status.DualStackStatistics.Total).Should(Equal(int32(0)))
					g.Expect(network.Status.DualStackStatistics.Available).Should(Equal(int32(254)))
					g.Expect(network.Status.DualStackStatistics.Used).Should(Equal(int32(0)))

				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Check status dynamic update", func() {
		It("Prepare", func() {
			By("create a new underlay network for checking status dynamic update")
			Expect(k8sClient.Create(context.Background(), underlayNetworkRender(underlayNetworkNameForStatus, 30))).NotTo(HaveOccurred())
		})

		It("Check node list after creating nodes", func() {
			const node1, node2, node3 = "node1-status", "node2-status", "node3-status"
			newNodes := []*corev1.Node{
				nodeRender(node1, map[string]string{
					"network": underlayNetworkNameForStatus,
				}),
				nodeRender(node2, map[string]string{
					"network": underlayNetworkNameForStatus,
				}),
				nodeRender(node3, map[string]string{
					"network": underlayNetworkNameForStatus,
				}),
			}

			By("creating nodes")
			for _, node := range newNodes {
				Expect(k8sClient.Create(context.Background(), node)).NotTo(HaveOccurred())
			}

			By("checking node list of status")
			Eventually(
				func(g Gomega) {
					network, err := utils.GetNetwork(context.Background(), k8sClient, underlayNetworkNameForStatus)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(network).NotTo(BeNil())

					g.Expect(network.Status.NodeList).To(HaveLen(3))
					g.Expect(network.Status.NodeList).To(ConsistOf(node1, node2, node3))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("removing nodes")
			for _, node := range newNodes {
				Expect(k8sClient.Delete(context.Background(), node)).NotTo(HaveOccurred())
			}
		})

		It("check subnet list and statistics after creating subnets", func() {
			const subnet1, subnet2, subnet3 = "subnet1-status", "subnet2-status", "subnet3-status"
			newSubnets := []*networkingv1.Subnet{
				subnetRender(subnet1, underlayNetworkNameForStatus, "192.168.57.0/24", pointer.Int32Ptr(57), true),
				subnetRender(subnet2, underlayNetworkNameForStatus, "192.168.58.0/24", pointer.Int32Ptr(58), true),
				subnetRender(subnet3, underlayNetworkNameForStatus, "fe81::0/120", pointer.Int32Ptr(59), false),
			}

			By("creating subnets")
			for _, subnet := range newSubnets {
				Expect(k8sClient.Create(context.Background(), subnet)).NotTo(HaveOccurred())
			}

			By("checking subnet list and statistics of status")
			Eventually(
				func(g Gomega) {
					network, err := utils.GetNetwork(context.Background(), k8sClient, underlayNetworkNameForStatus)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(network).NotTo(BeNil())

					g.Expect(network.Status.SubnetList).To(HaveLen(3))
					g.Expect(network.Status.SubnetList).To(ConsistOf(subnet1, subnet2, subnet3))

					g.Expect(network.Status.Statistics).NotTo(BeNil())
					g.Expect(network.Status.Statistics.Total).Should(Equal(int32(253 * 2)))
					g.Expect(network.Status.Statistics.Available).Should(Equal(int32(253 * 2)))
					g.Expect(network.Status.Statistics.Used).Should(Equal(int32(0)))

					g.Expect(network.Status.IPv6Statistics).NotTo(BeNil())
					g.Expect(network.Status.IPv6Statistics.Total).Should(Equal(int32(255)))
					g.Expect(network.Status.IPv6Statistics.Available).Should(Equal(int32(255)))
					g.Expect(network.Status.IPv6Statistics.Used).Should(Equal(int32(0)))

					g.Expect(network.Status.DualStackStatistics).NotTo(BeNil())
					g.Expect(network.Status.DualStackStatistics.Total).Should(Equal(int32(0)))
					g.Expect(network.Status.DualStackStatistics.Available).Should(Equal(int32(255)))
					g.Expect(network.Status.DualStackStatistics.Used).Should(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("removing subnets")
			for _, subnet := range newSubnets {
				Expect(k8sClient.Delete(context.Background(), subnet)).NotTo(HaveOccurred())
			}
		})

		It("Recycle", func() {
			By("remove overlay network for status test")
			network := overlayNetworkRender(underlayNetworkNameForStatus, 30)
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())

			By("check overlay network removed from apiserver")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(), types.NamespacedName{
						Name: underlayNetworkNameForStatus,
					}, network)

					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
