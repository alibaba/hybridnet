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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
)

var _ = Describe("Node controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("Initialization check", func() {
		It("Checking initialized node attachment labels", func() {
			By("check node1 attachment labels")
			Eventually(
				func(g Gomega) {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						node)).NotTo(HaveOccurred())

					g.Expect(node.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(node.Labels).To(HaveKey(constants.LabelOverlayNetworkAttachment))
					g.Expect(node.Labels).NotTo(HaveKey(constants.LabelBGPNetworkAttachment))

				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check node2 attachment labels")
			Eventually(
				func(g Gomega) {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						node)).NotTo(HaveOccurred())

					g.Expect(node.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(node.Labels).To(HaveKey(constants.LabelOverlayNetworkAttachment))
					g.Expect(node.Labels).NotTo(HaveKey(constants.LabelBGPNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check node3 attachment labels")
			Eventually(
				func(g Gomega) {
					node := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node3Name,
						},
						node)).NotTo(HaveOccurred())

					g.Expect(node.Labels).NotTo(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(node.Labels).To(HaveKey(constants.LabelOverlayNetworkAttachment))
					g.Expect(node.Labels).NotTo(HaveKey(constants.LabelBGPNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Dynamic update check", func() {
		It("Check node attachment label update after changing network selector", func() {
			By("create test underlay network")
			networkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			anotherNetworkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			network := underlayNetworkRender(networkName, 200)
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

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

			By("checking node underlay attachment label")
			Eventually(
				func(g Gomega) {
					currentNode1 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						currentNode1)).NotTo(HaveOccurred())

					g.Expect(currentNode1.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))

					currentNode2 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						currentNode2)).NotTo(HaveOccurred())

					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelUnderlayNetworkAttachment))
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

			By("checking node underlay attachment label update")
			Eventually(
				func(g Gomega) {
					currentNode1 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						currentNode1)).NotTo(HaveOccurred())

					g.Expect(currentNode1.Labels).NotTo(HaveKey(constants.LabelUnderlayNetworkAttachment))

					currentNode2 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						currentNode2)).NotTo(HaveOccurred())

					g.Expect(currentNode2.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
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

			By("checking node underlay attachment label update again")
			Eventually(
				func(g Gomega) {
					currentNode1 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node1Name,
						},
						currentNode1)).NotTo(HaveOccurred())

					g.Expect(currentNode1.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))

					currentNode2 := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: node2Name,
						},
						currentNode2)).NotTo(HaveOccurred())

					g.Expect(currentNode2.Labels).NotTo(HaveKey(constants.LabelUnderlayNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test objects")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node1)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node2)).NotTo(HaveOccurred())
		})

		It("Check node attachment label update after changing node related label", func() {
			By("create test underlay network")
			networkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			network := underlayNetworkRender(networkName, 200)
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("create test node")
			nodeName := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node := nodeRender(nodeName, map[string]string{
				"network": networkName,
			})
			Expect(k8sClient.Create(context.Background(), node)).NotTo(HaveOccurred())

			By("checking node underlay attachment label")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update node label about network selector")
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

			By("checking node underlay attachment label update")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelUnderlayNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("rollback network selector label on node")
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

			By("checking node underlay attachment label update again")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test objects")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), node)).NotTo(HaveOccurred())
		})

		It("Check node attachment label update after creating globalBGP network", func() {
			By("create test underlay network of BGP mode")
			networkName := fmt.Sprintf("test-underlay-network-%s", uuid.NewUUID())
			network := underlayNetworkRender(networkName, 123)
			network.Spec.Mode = networkingv1.NetworkModeBGP
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("create test node")
			nodeName := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node := nodeRender(nodeName, map[string]string{
				"network": networkName,
			})
			Expect(k8sClient.Create(context.Background(), node)).NotTo(HaveOccurred())

			By("checking node underlay and globalBGP attachment label")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelBGPNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("create test globalBGP network")
			globalBGPNetworkName := fmt.Sprintf("test-globalbgp-network-%s", uuid.NewUUID())
			globalBGPNetwork := globalBGPNetworkRender(globalBGPNetworkName, 210)
			Expect(k8sClient.Create(context.Background(), globalBGPNetwork)).NotTo(HaveOccurred())

			By("checking node globalBGP attachment label updated")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(currentNode.Labels).To(HaveKey(constants.LabelBGPNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove test globalBGP network")
			Expect(k8sClient.Delete(context.Background(), globalBGPNetwork)).NotTo(HaveOccurred())

			By("checking node globalBGP attachment label removed")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).To(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelBGPNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove test underlay network")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())

			By("checking node underlay and globalBGP attachment label removed")
			Eventually(
				func(g Gomega) {
					currentNode := &corev1.Node{}
					g.Expect(k8sClient.Get(
						context.Background(),
						types.NamespacedName{
							Name: nodeName,
						},
						currentNode)).NotTo(HaveOccurred())

					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelUnderlayNetworkAttachment))
					g.Expect(currentNode.Labels).NotTo(HaveKey(constants.LabelBGPNetworkAttachment))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test node")
			Expect(k8sClient.Delete(context.Background(), node)).NotTo(HaveOccurred())
		})
	})

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
