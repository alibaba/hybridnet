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

	"github.com/alibaba/hybridnet/pkg/utils/transform"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

var _ = Describe("Subnet status controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("Initialization check", func() {
		It("Check initialized subnet status", func() {
			By("check initialized underlay subnet status")
			Eventually(
				func(g Gomega) {
					subnet, err := utils.GetSubnet(context.Background(), k8sClient, underlaySubnetName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(subnet).NotTo(BeNil())

					g.Expect(subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - gatewayAddress - broadcastAddress))
					g.Expect(subnet.Status.Available).To(Equal(subnet.Status.Total))
					g.Expect(subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check initialized overlay ipv4 subnet status")
			Eventually(
				func(g Gomega) {
					subnet, err := utils.GetSubnet(context.Background(), k8sClient, overlayIPv4SubnetName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(subnet).NotTo(BeNil())

					g.Expect(subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - broadcastAddress))
					g.Expect(subnet.Status.Available).To(Equal(subnet.Status.Total))
					g.Expect(subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check initialized overlay ipv6 subnet status")
			Eventually(
				func(g Gomega) {
					subnet, err := utils.GetSubnet(context.Background(), k8sClient, overlayIPv6SubnetName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(subnet).NotTo(BeNil())

					// IPv6 does not have broadcast address
					g.Expect(subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress))
					g.Expect(subnet.Status.Available).To(Equal(subnet.Status.Total))
					g.Expect(subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Dynamic status update check", func() {
		var networkName, ipv4SubnetName, ipv6SubnetName string

		BeforeEach(func() {
			networkName = fmt.Sprintf("test-network-%s", uuid.NewUUID())
			ipv4SubnetName = fmt.Sprintf("test-ipv4-subnet-%s", uuid.NewUUID())
			ipv6SubnetName = fmt.Sprintf("test-ipv6-subnet-%s", uuid.NewUUID())

			By("creat test underlay network")
			network := underlayNetworkRender(networkName, 345)
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("create test subnet")
			ipv4Subnet := subnetRender(ipv4SubnetName, networkName, "170.16.0.0/24", nil, true)
			ipv6Subnet := subnetRender(ipv6SubnetName, networkName, "fe20::0/120", nil, false)
			Expect(k8sClient.Create(context.Background(), ipv4Subnet)).NotTo(HaveOccurred())
			Expect(k8sClient.Create(context.Background(), ipv6Subnet)).NotTo(HaveOccurred())
		})

		It("Check subnet status update after updating spec", func() {
			By("check subnet status initialization")
			Eventually(
				func(g Gomega) {
					currentIPv4Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv4SubnetName,
						},
						currentIPv4Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv4Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - broadcastAddress - gatewayAddress))
					g.Expect(currentIPv4Subnet.Status.Available).To(Equal(currentIPv4Subnet.Status.Total))
					g.Expect(currentIPv4Subnet.Status.Used).To(Equal(int32(0)))

					currentIPv6Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv6SubnetName,
						},
						currentIPv6Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv6Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress))
					g.Expect(currentIPv6Subnet.Status.Available).To(Equal(currentIPv6Subnet.Status.Total))
					g.Expect(currentIPv6Subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update start and end of ipv4 subnet")
			ipv4Subnet := &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: ipv4SubnetName,
				},
			}
			Expect(controllerutil.CreateOrPatch(context.Background(),
				k8sClient,
				ipv4Subnet,
				func() error {
					ipv4Subnet.Spec.Range.Start = "170.16.0.21"
					ipv4Subnet.Spec.Range.End = "170.16.0.50"
					return nil
				})).Error().NotTo(HaveOccurred())

			By("check ipv4 subnet status")
			Eventually(
				func(g Gomega) {
					currentIPv4Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv4SubnetName,
						},
						currentIPv4Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv4Subnet.Status.Total).To(Equal(int32(30)))
					g.Expect(currentIPv4Subnet.Status.Available).To(Equal(currentIPv4Subnet.Status.Total))
					g.Expect(currentIPv4Subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update reserved ips of ipv6 subnet")
			ipv6Subnet := &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: ipv6SubnetName,
				},
			}
			reservedIPs := []string{
				"fe20::10",
				"fe20::11",
				"fe20::12",
				"fe20::13",
				"fe20::14",
			}
			Expect(controllerutil.CreateOrPatch(context.Background(),
				k8sClient,
				ipv6Subnet,
				func() error {
					ipv6Subnet.Spec.Range.ReservedIPs = reservedIPs
					return nil
				})).Error().NotTo(HaveOccurred())

			By("check ipv6 subnet status")
			Eventually(
				func(g Gomega) {
					currentIPv6Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv6SubnetName,
						},
						currentIPv6Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv6Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - int32(len(reservedIPs))))
					g.Expect(currentIPv6Subnet.Status.Available).To(Equal(currentIPv6Subnet.Status.Total))
					g.Expect(currentIPv6Subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update excluded ips of ipv6 subnet")
			ipv6Subnet = &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: ipv6SubnetName,
				},
			}
			excludedIPs := []string{
				"fe20::64",
				"fe20::65",
				"fe20::66",
				"fe20::67",
				"fe20::68",
			}
			Expect(controllerutil.CreateOrPatch(context.Background(),
				k8sClient,
				ipv6Subnet,
				func() error {
					ipv6Subnet.Spec.Range.ExcludeIPs = excludedIPs
					return nil
				})).Error().NotTo(HaveOccurred())

			By("check ipv6 subnet status")
			Eventually(
				func(g Gomega) {
					currentIPv6Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv6SubnetName,
						},
						currentIPv6Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv6Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - int32(len(reservedIPs)) - int32(len(excludedIPs))))
					g.Expect(currentIPv6Subnet.Status.Available).To(Equal(currentIPv6Subnet.Status.Total))
					g.Expect(currentIPv6Subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Check subnet status update after ip allocation", func() {
			By("create test node")
			nodeName := fmt.Sprintf("test-node-%s", uuid.NewUUID())
			node := nodeRender(nodeName, map[string]string{
				"network": networkName,
			})
			Expect(k8sClient.Create(context.Background(), node)).NotTo(HaveOccurred())

			var ipv4Available, ipv6Available int32
			By("check subnet status initialization and record available")
			Eventually(
				func(g Gomega) {
					currentIPv4Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv4SubnetName,
						},
						currentIPv4Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv4Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - broadcastAddress - gatewayAddress))
					g.Expect(currentIPv4Subnet.Status.Available).To(Equal(currentIPv4Subnet.Status.Total))
					g.Expect(currentIPv4Subnet.Status.Used).To(Equal(int32(0)))
					// record available of ipv4 subnet
					ipv4Available = currentIPv4Subnet.Status.Available

					currentIPv6Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv6SubnetName,
						},
						currentIPv6Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv6Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress))
					g.Expect(currentIPv6Subnet.Status.Available).To(Equal(currentIPv6Subnet.Status.Total))
					g.Expect(currentIPv6Subnet.Status.Used).To(Equal(int32(0)))
					// record available of ipv6 subnet
					ipv6Available = currentIPv6Subnet.Status.Available
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("create a single pod requiring DualStack addresses")
			podName := fmt.Sprintf("test-pod-%s", uuid.NewUUID())
			pod := simplePodRender(podName, nodeName)
			pod.Annotations = map[string]string{
				constants.AnnotationIPFamily: "DualStack",
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check ip instance count of DualStack allocation")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check subnet status update after ip allocation")
			Eventually(
				func(g Gomega) {
					currentIPv4Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv4SubnetName,
						},
						currentIPv4Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv4Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - broadcastAddress - gatewayAddress))
					g.Expect(currentIPv4Subnet.Status.Available).To(Equal(ipv4Available - int32(1)))
					g.Expect(currentIPv4Subnet.Status.Used).To(Equal(int32(1)))
					// gateway is set to the first ip by default in subnetRender
					g.Expect(currentIPv4Subnet.Status.LastAllocatedIP).To(Equal("170.16.0.2"))

					currentIPv6Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv6SubnetName,
						},
						currentIPv6Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv6Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress))
					g.Expect(currentIPv6Subnet.Status.Available).To(Equal(ipv6Available - int32(1)))
					g.Expect(currentIPv6Subnet.Status.Used).To(Equal(int32(1)))
					g.Expect(currentIPv6Subnet.Status.LastAllocatedIP).To(Equal("fe20::1"))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod and related ip instances")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(pod.Name),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      pod.Name,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check subnet status update after ip recycle")
			Eventually(
				func(g Gomega) {
					currentIPv4Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv4SubnetName,
						},
						currentIPv4Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv4Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress - broadcastAddress - gatewayAddress))
					g.Expect(currentIPv4Subnet.Status.Available).To(Equal(ipv4Available))
					g.Expect(currentIPv4Subnet.Status.Used).To(Equal(int32(0)))

					currentIPv6Subnet := &networkingv1.Subnet{}
					g.Expect(k8sClient.Get(context.Background(),
						types.NamespacedName{
							Name: ipv6SubnetName,
						},
						currentIPv6Subnet,
					)).NotTo(HaveOccurred())

					g.Expect(currentIPv6Subnet.Status.Total).To(Equal(basicIPQuantity - networkAddress))
					g.Expect(currentIPv6Subnet.Status.Available).To(Equal(ipv6Available))
					g.Expect(currentIPv6Subnet.Status.Used).To(Equal(int32(0)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove test node")
			Expect(k8sClient.Delete(context.Background(), node)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("remove test subnet")
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: ipv4SubnetName,
				},
			})).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: ipv6SubnetName,
				},
			})).NotTo(HaveOccurred())

			By("remote test network")
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name: networkName,
				},
			}))
		})
	})

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
