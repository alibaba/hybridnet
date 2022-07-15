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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
)

var _ = Describe("Quota controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("Initialization check", func() {
		It("Checking initialized node quota labels", func() {
			By("check node1 quota labels")
			Eventually(
				func(g Gomega) {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						node)).NotTo(HaveOccurred())

					g.Expect(node.Labels).To(SatisfyAll(
						HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty),
						HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaEmpty),
						HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaEmpty),
					))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check node2 quota labels")
			Eventually(
				func(g Gomega) {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						node)).NotTo(HaveOccurred())

					g.Expect(node.Labels).To(SatisfyAll(
						HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty),
						HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaEmpty),
						HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaEmpty),
					))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check node3 quota labels")
			Eventually(
				func(g Gomega) {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node3Name,
						},
						node)).NotTo(HaveOccurred())

					g.Expect(node.Labels).To(SatisfyAll(
						Not(HaveKey(constants.LabelIPv4AddressQuota)),
						Not(HaveKey(constants.LabelIPv6AddressQuota)),
						Not(HaveKey(constants.LabelDualStackAddressQuota)),
					))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Dynamic update check", func() {
		It("Check node quota label update when changing network selector", func() {
			By("create test underlay network")
			networkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			anotherNetworkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			network := underlayNetworkRender(networkName, 200)
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("create test subnets")
			subnets := []*networkingv1.Subnet{
				subnetRender(fmt.Sprintf("test-subnet-%s", uuid.NewUUID()), networkName, "192.168.123.0/24", nil, true),
				subnetRender(fmt.Sprintf("test-subnet-%s", uuid.NewUUID()), networkName, "fe92::0/120", pointer.Int32Ptr(92), false),
			}
			for _, subnet := range subnets {
				Expect(k8sClient.Create(context.Background(), subnet)).NotTo(HaveOccurred())
			}

			By("create test nodes")
			node1Name := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node2Name := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node1 := nodeRender(node1Name, map[string]string{
				"network": networkName,
			})
			node2 := nodeRender(node2Name, map[string]string{
				"network": anotherNetworkName,
			})
			Expect(k8sClient.Create(context.Background(), node1)).NotTo(HaveOccurred())
			Expect(k8sClient.Create(context.Background(), node2)).NotTo(HaveOccurred())

			By("checking node quota label")
			Eventually(
				func(g Gomega) {
					currentNode1 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						currentNode1)).NotTo(HaveOccurred())

					g.Expect(currentNode1.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode1.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode1.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaNonEmpty))

					currentNode2 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						currentNode2)).NotTo(HaveOccurred())

					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelIPv4AddressQuota))
					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelIPv6AddressQuota))
					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelDualStackAddressQuota))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update underlay network selector to another name")
			Expect(
				func() error {
					tempNetwork := &networkingv1.Network{
						ObjectMeta: metav1.ObjectMeta{
							Name: networkName,
						},
					}
					_, err := controllerutil.CreateOrPatch(context.Background(), k8sClient, tempNetwork, func() error {
						tempNetwork.Spec.NodeSelector = map[string]string{
							"network": anotherNetworkName,
						}
						return nil
					})
					return err
				}(),
			).NotTo(HaveOccurred())

			By("checking node quota label update on different nodes after changing network selector")
			Eventually(
				func(g Gomega) {
					currentNode1 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						currentNode1)).NotTo(HaveOccurred())

					g.Expect(currentNode1.Labels).NotTo(HaveKey(constants.LabelIPv4AddressQuota))
					g.Expect(currentNode1.Labels).NotTo(HaveKey(constants.LabelIPv6AddressQuota))
					g.Expect(currentNode1.Labels).NotTo(HaveKey(constants.LabelDualStackAddressQuota))

					currentNode2 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						currentNode2)).NotTo(HaveOccurred())

					g.Expect(currentNode2.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode2.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode2.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaNonEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("rollback underlay network selector to its own name")
			Expect(
				func() error {
					tempNetwork := &networkingv1.Network{
						ObjectMeta: metav1.ObjectMeta{
							Name: networkName,
						},
					}
					_, err := controllerutil.CreateOrPatch(context.Background(), k8sClient, tempNetwork, func() error {
						tempNetwork.Spec.NodeSelector = map[string]string{
							"network": networkName,
						}
						return nil
					})
					return err
				}(),
			).NotTo(HaveOccurred())

			By("checking node quota label update again back on different nodes")
			Eventually(
				func(g Gomega) {
					currentNode1 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						currentNode1)).NotTo(HaveOccurred())

					g.Expect(currentNode1.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode1.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode1.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaNonEmpty))

					currentNode2 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						currentNode2)).NotTo(HaveOccurred())

					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelIPv4AddressQuota))
					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelIPv6AddressQuota))
					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelDualStackAddressQuota))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test objects")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node1)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node2)).NotTo(HaveOccurred())
			for _, subnet := range subnets {
				Expect(k8sClient.Delete(context.Background(), subnet)).NotTo(HaveOccurred())
			}
		})

		It("Check node quota label update when changing node selector label", func() {
			By("create test underlay network")
			networkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			network := underlayNetworkRender(networkName, 500)
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("create test subnets")
			subnets := []*networkingv1.Subnet{
				subnetRender(fmt.Sprintf("test-subnet-%s", uuid.NewUUID()), networkName, "192.168.124.0/24", nil, true),
				subnetRender(fmt.Sprintf("test-subnet-%s", uuid.NewUUID()), networkName, "fe93::0/120", pointer.Int32Ptr(93), false),
			}
			for _, subnet := range subnets {
				Expect(k8sClient.Create(context.Background(), subnet)).NotTo(HaveOccurred())
			}

			By("create test node")
			nodeName := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node := nodeRender(nodeName, map[string]string{
				"network": networkName,
			})
			Expect(k8sClient.Create(context.Background(), node)).NotTo(HaveOccurred())

			By("checking node quota label")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaNonEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update node selector label")
			Expect(
				func() error {
					tempNode := &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: nodeName,
						},
					}
					_, err := controllerutil.CreateOrPatch(context.Background(), k8sClient, tempNode, func() error {
						tempNode.Labels["network"] = "unexpected"
						return nil
					})
					return err
				}(),
			).NotTo(HaveOccurred())

			By("checking node quota label update after non matching any network")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelIPv4AddressQuota))
					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelIPv6AddressQuota))
					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelDualStackAddressQuota))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("rollback node selector label")
			Expect(
				func() error {
					tempNode := &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: nodeName,
						},
					}
					_, err := controllerutil.CreateOrPatch(context.Background(), k8sClient, tempNode, func() error {
						tempNode.Labels["network"] = networkName
						return nil
					})
					return err
				}(),
			).NotTo(HaveOccurred())

			By("checking node quota label update again after rematching test network")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaNonEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test objects")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node)).NotTo(HaveOccurred())
			for _, subnet := range subnets {
				Expect(k8sClient.Delete(context.Background(), subnet)).NotTo(HaveOccurred())
			}
		})

		It("Check node quota label update when network statistics change", func() {
			By("create test underlay network")
			networkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			network := underlayNetworkRender(networkName, 501)
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("create test node")
			nodeName := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node := nodeRender(nodeName, map[string]string{
				"network": networkName,
			})
			Expect(k8sClient.Create(context.Background(), node)).NotTo(HaveOccurred())

			By("checking node quota label when network have no subnets")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("create test ipv4 subnets")
			ipv4Subnet := subnetRender(fmt.Sprintf("test-ipv4-subnet-%s", uuid.NewUUID()), networkName, "192.168.124.0/24", nil, true)
			Expect(k8sClient.Create(context.Background(), ipv4Subnet)).NotTo(HaveOccurred())

			By("checking node quota label when network have one ipv4 subnet")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("create test ipv6 subnets")
			ipv6Subnet := subnetRender(fmt.Sprintf("test-ipv6-subnet-%s", uuid.NewUUID()), networkName, "fe93::0/120", pointer.Int32Ptr(93), false)
			Expect(k8sClient.Create(context.Background(), ipv6Subnet)).NotTo(HaveOccurred())

			By("checking node quota label when network have both ipv4 and ipv6 subnets")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaNonEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove test ipv4 subnets")
			Expect(k8sClient.Delete(context.Background(), ipv4Subnet)).NotTo(HaveOccurred())

			By("checking node quota label when network have only one ipv6 subnets")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaNonEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove test ipv6 subnets")
			Expect(k8sClient.Delete(context.Background(), ipv6Subnet)).NotTo(HaveOccurred())

			By("checking node quota label when all subnets of network have been removed")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv4AddressQuota, constants.QuotaEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelIPv6AddressQuota, constants.QuotaEmpty))
					g.Expect(currentNode.Labels).To(HaveKeyWithValue(constants.LabelDualStackAddressQuota, constants.QuotaEmpty))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test objects")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node)).NotTo(HaveOccurred())
		})
	})

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
