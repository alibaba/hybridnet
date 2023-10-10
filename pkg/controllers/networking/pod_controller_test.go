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
	"math/rand"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/utils/transform"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("Pod controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("IP allocation for single pod", func() {
		var podName string

		BeforeEach(func() {
			podName = fmt.Sprintf("pod-%s", uuid.NewUUID())
		})

		It("Allocate IPv4 address of underlay network for single pod", func() {
			By("create single pod on a node who has underlay network")
			pod := simplePodRender(podName, node1Name)
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check IPv4 address allocation")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: "Pod",
						Name: pod.Name,
						UID:  pod.UID,
					}))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Allocate IPv6 address of overlay network for single pod", func() {
			By("create single pod requiring overlay network and IPv6 address")
			pod := simplePodRender(podName, node3Name)
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Overlay",
				constants.AnnotationIPFamily:    "IPv6",
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check IPv6 address allocation")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: "Pod",
						Name: pod.Name,
						UID:  pod.UID,
					}))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Allocate DualStack addresses of overlay network for single pod", func() {
			By("create a single pod requiring overlay network and DualStack addresses")
			pod := simplePodRender(podName, node3Name)
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Overlay",
				constants.AnnotationIPFamily:    "DualStack",
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check DualStack addresses allocation")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					networkingv1.SortIPInstancePointerSlice(ipInstances)
					ipInstanceIPv4 := ipInstances[0]
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: "Pod",
						Name: pod.Name,
						UID:  pod.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					ipInstanceIPv6 := ipInstances[1]
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: "Pod",
						Name: pod.Name,
						UID:  pod.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())
		})
	})

	Context("IP retain for single stateful pod", func() {
		var podName string
		var ownerReference metav1.OwnerReference

		BeforeEach(func() {
			podName = fmt.Sprintf("pod-%d", rand.Intn(10))
			ownerReference = statefulOwnerReferenceRender()
		})

		It("Allocate and retain IPv4 address of underlay network for single stateful pod", func() {
			By("create a stateful pod requiring IPv4 address")
			var ipInstanceName string
			pod := simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the first allocated IPv4 address")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					ipInstanceName = ipInstance.Name
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("check the allocated IPv4 address is reserved")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			// TODO: check status in IPAM manager

			By("make sure pod deleted")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("recreate the stateful pod")
			pod = simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check the allocated IPv4 address is retained and reused")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Allocate and retain IPv6 address of overlay network for single stateful pod", func() {
			By("create a stateful pod requiring IPv6 address and overlay network")
			var ipInstanceName string
			pod := simplePodRender(podName, node3Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Overlay",
				constants.AnnotationIPFamily:    "IPv6",
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the first allocated IPv6 address")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					ipInstanceName = ipInstance.Name
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node3Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("check the allocated IPv6 address is reserved")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			// TODO: check status in IPAM manager

			By("make sure pod deleted")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("recreate the stateful pod on another node")
			pod = simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Overlay",
				constants.AnnotationIPFamily:    "IPv6",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check the allocated IPv6 address is retained and reused")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Allocate and retain DualStack addresses of overlay network for single stateful pod", func() {
			By("create a stateful pod requiring DualStack addresses and overlay network")
			var ipInstanceIPv4Name string
			var ipInstanceIPv6Name string
			pod := simplePodRender(podName, node3Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Overlay",
				constants.AnnotationIPFamily:    "DualStack",
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the first allocated DualStack addresses")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					// sort by ip family order
					networkingv1.SortIPInstancePointerSlice(ipInstances)

					// check IPv4 IPInstance
					ipInstanceIPv4 := ipInstances[0]
					ipInstanceIPv4Name = ipInstanceIPv4.Name
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.NodeName).To(Equal(node3Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstanceIPv4.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					// check IPv6 IPInstance
					ipInstanceIPv6 := ipInstances[1]
					ipInstanceIPv6Name = ipInstanceIPv6.Name
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.NodeName).To(Equal(node3Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx = *ipInstanceIPv6.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))

					// check MAC address
					g.Expect(ipInstanceIPv4.Spec.Address.MAC).To(Equal(ipInstanceIPv6.Spec.Address.MAC))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("check the allocated DualStack addresses are reserved")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					// sort by ip family order
					networkingv1.SortIPInstancePointerSlice(ipInstances)

					// check IPv4 IPInstance
					ipInstanceIPv4 := ipInstances[0]
					g.Expect(ipInstanceIPv4.Name).To(Equal(ipInstanceIPv4Name))
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstanceIPv4.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					// check IPv6 IPInstance
					ipInstanceIPv6 := ipInstances[1]
					g.Expect(ipInstanceIPv6.Name).To(Equal(ipInstanceIPv6Name))
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx = *ipInstanceIPv6.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))

					// check MAC address
					g.Expect(ipInstanceIPv4.Spec.Address.MAC).To(Equal(ipInstanceIPv6.Spec.Address.MAC))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			// TODO: check status in IPAM manager

			By("make sure pod deleted")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("recreate the stateful pod on another node")
			pod = simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Overlay",
				constants.AnnotationIPFamily:    "DualStack",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check the allocated DualStack addresses are retained and reused")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					// sort by ip family order
					networkingv1.SortIPInstancePointerSlice(ipInstances)

					// check IPv4 IPInstance
					ipInstanceIPv4 := ipInstances[0]
					g.Expect(ipInstanceIPv4.Name).To(Equal(ipInstanceIPv4Name))
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstanceIPv4.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					// check IPv6 IPInstance
					ipInstanceIPv6 := ipInstances[1]
					g.Expect(ipInstanceIPv6.Name).To(Equal(ipInstanceIPv6Name))
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx = *ipInstanceIPv6.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))

					// check MAC address
					g.Expect(ipInstanceIPv4.Spec.Address.MAC).To(Equal(ipInstanceIPv6.Spec.Address.MAC))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Check ip reserved only after pod is not running", func() {
			By("create a stateful pod requiring IPv4 address")
			var ipInstanceName string
			pod := simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the first allocated IPv4 address")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					ipInstanceName = ipInstance.Name
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("update to make sure pod is running")
			patch := client.MergeFrom(pod.DeepCopy())
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "test",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Time{Time: time.Now()},
						},
					},
				},
			}
			Expect(k8sClient.Status().Patch(context.Background(), pod, patch)).NotTo(HaveOccurred())

			By("try to delete stateful pod into terminating state")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("check allocated IPv4 address still not reserved after 30s")
			Consistently(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Binding.NodeName).NotTo(BeEmpty())

				}).
				WithTimeout(30 * time.Second).
				WithPolling(5 * time.Second).
				Should(Succeed())

			By("update to make sure pod is not running")
			patch = client.MergeFrom(pod.DeepCopy())
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "test",
					State: corev1.ContainerState{
						Running: nil,
					},
				},
			}
			Expect(k8sClient.Status().Patch(context.Background(), pod, patch)).NotTo(HaveOccurred())

			By("check the allocated IPv4 address is reserved")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("make sure pod deleted")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("recreate the stateful pod")
			pod = simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check the allocated IPv4 address is retained and reused")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(BeElementOf(underlaySubnetName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("make sure test ip instances cleaned up")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Assign network through labels/annotations", func() {
		var podName string

		BeforeEach(func() {
			podName = fmt.Sprintf("test-pod-%s", uuid.NewUUID())
		})

		It("Config pod with specified underlay network in pod annotations", func() {
			By("create pod with specified underlay network in annotation")
			pod := simplePodRender(podName, node1Name)
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: underlayNetworkName,
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with specified overlay network type in pod labels", func() {
			By("create pod with specified overlay network type specified in label")
			pod := simplePodRender(podName, node1Name)
			pod.Labels = map[string]string{
				constants.LabelSpecifiedNetwork: overlayNetworkName,
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with specified overlay network in namespace annotations", func() {
			By("update assigned network in namespace annotations")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					if len(ns.Annotations) == 0 {
						ns.Annotations = map[string]string{}
					}
					ns.Annotations[constants.AnnotationSpecifiedNetwork] = overlayNetworkName
					return nil
				})).
				Error().
				NotTo(HaveOccurred())

			By("create pod with no labels or annotations")
			pod := simplePodRender(podName, node1Name)
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("clean assigned network in namespace annotations")
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					ns.Annotations = map[string]string{}
					return nil
				})).
				Error().
				NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(),
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      podName,
					},
				},
				client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("clean up test ip instances")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

		})
	})

	Context("Assign subnet through labels/annotations", func() {
		var podName string
		var networkName = fmt.Sprintf("network-test-%s", uuid.NewUUID())
		var subnet1Name = fmt.Sprintf("subnet-test-%s", uuid.NewUUID())
		var subnet2Name = fmt.Sprintf("subnet-test-%s", uuid.NewUUID())
		var subnet3Name = fmt.Sprintf("subnet-test-%s", uuid.NewUUID())
		var nodeName = fmt.Sprintf("node-test-%s", uuid.NewUUID())

		BeforeEach(func() {
			podName = fmt.Sprintf("test-pod-%s", uuid.NewUUID())
		})

		It("Create one network with multiple subnets", func() {
			By("create test underlay network")
			Expect(k8sClient.Create(context.Background(),
				underlayNetworkRender(networkName, 33))).
				NotTo(HaveOccurred())

			By("create test subnets")
			Expect(k8sClient.Create(context.Background(),
				subnetRender(
					subnet1Name,
					networkName,
					"200.200.0.0/24",
					nil,
					true,
				))).NotTo(HaveOccurred())
			Expect(k8sClient.Create(context.Background(),
				subnetRender(
					subnet2Name,
					networkName,
					"200.200.1.0/24",
					nil,
					true,
				))).NotTo(HaveOccurred())
			Expect(k8sClient.Create(context.Background(),
				subnetRender(
					subnet3Name,
					networkName,
					"200.200.2.0/24",
					nil,
					true,
				))).NotTo(HaveOccurred())

			By("create test node binding on test network")
			Expect(k8sClient.Create(context.Background(),
				nodeRender(
					nodeName,
					map[string]string{
						"network": networkName,
					},
				))).NotTo(HaveOccurred())
		})

		It("Config pod with specified subnet 1 in pod annotations", func() {
			By("create pod with special annotations")
			pod := simplePodRender(podName, nodeName)
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: networkName,
				constants.AnnotationSpecifiedSubnet:  subnet1Name,
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(networkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(subnet1Name))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with specified subnet 2 in pod annotations", func() {
			By("create pod with special annotations")
			pod := simplePodRender(podName, nodeName)
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: networkName,
				constants.AnnotationSpecifiedSubnet:  subnet2Name,
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(networkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(subnet2Name))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with specified subnet 3 in pod annotations", func() {
			By("create pod with special annotations")
			pod := simplePodRender(podName, nodeName)
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: networkName,
				constants.AnnotationSpecifiedSubnet:  subnet3Name,
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(networkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(subnet3Name))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with specified subnet 2 in namespace annotations", func() {
			By("update assigned subnet in namespace annotations")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					if len(ns.Annotations) == 0 {
						ns.Annotations = map[string]string{}
					}
					ns.Annotations[constants.AnnotationSpecifiedNetwork] = networkName
					ns.Annotations[constants.AnnotationSpecifiedSubnet] = subnet2Name
					return nil
				})).
				Error().
				NotTo(HaveOccurred())

			By("create pod with no labels or annotations")
			pod := simplePodRender(podName, nodeName)
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(networkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(subnet2Name))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("clean assigned subnet in namespace annotations")
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					ns.Annotations = map[string]string{}
					return nil
				})).
				Error().
				NotTo(HaveOccurred())
		})

		It("remove test objects", func() {
			By("remove test node")
			Expect(k8sClient.Delete(context.Background(), &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
			}))

			By("remove test subnets")
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: subnet1Name,
				},
			})).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: subnet2Name,
				},
			})).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: subnet3Name,
				},
			})).NotTo(HaveOccurred())

			By("remote test network")
			Expect(k8sClient.Delete(context.Background(), &networkingv1.Network{
				ObjectMeta: metav1.ObjectMeta{
					Name: networkName,
				},
			}))
		})

		AfterEach(func() {
			By("remove the test pod")
			Expect(client.IgnoreNotFound(
				k8sClient.Delete(context.Background(),
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      podName,
						},
					},
					client.GracePeriodSeconds(0)))).NotTo(HaveOccurred())

			By("clean up test ip instances")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

		})
	})

	Context("Assign network-type through labels/annotations", func() {
		var podName string

		BeforeEach(func() {
			podName = fmt.Sprintf("test-pod-%s", uuid.NewUUID())
		})

		It("Config pod with underlay network type in pod annotations", func() {
			By("create pod with underlay network type specified in annotation")
			pod := simplePodRender(podName, node1Name)
			pod.Annotations = map[string]string{
				constants.AnnotationNetworkType: "Underlay",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(underlayNetworkName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with overlay network type in pod labels", func() {
			By("create pod with overlay network type specified in label")
			pod := simplePodRender(podName, node1Name)
			pod.Labels = map[string]string{
				constants.LabelNetworkType: "Overlay",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with overlay network type in namespace annotations", func() {
			By("update assigned network type in namespace annotations")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					if len(ns.Annotations) == 0 {
						ns.Annotations = map[string]string{
							constants.AnnotationNetworkType: "overlay",
						}
					} else {
						ns.Annotations[constants.AnnotationNetworkType] = "overlay"
					}
					return nil
				})).
				Error().
				NotTo(HaveOccurred())

			By("create pod with no labels or annotations")
			pod := simplePodRender(podName, node1Name)
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("clean assigned network type in namespace annotations")
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					ns.Annotations = map[string]string{}
					return nil
				})).
				Error().
				NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(),
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      podName,
					},
				},
				client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("clean up test ip instances")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

		})
	})

	Context("Assign ip-family through annotations", func() {
		var podName string

		BeforeEach(func() {
			podName = fmt.Sprintf("test-pod-%s", uuid.NewUUID())
		})

		It("Config pod with IPv4 family in pod annotations", func() {
			By("create pod with IPv4 family specified in annotation")
			pod := simplePodRender(podName, node1Name)
			pod.Annotations = map[string]string{
				constants.AnnotationIPFamily:    "ipv4",
				constants.AnnotationNetworkType: "overlay",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with IPv6 family in pod annotations", func() {
			By("create pod with IPv6 family specified in annotation")
			pod := simplePodRender(podName, node1Name)
			pod.Annotations = map[string]string{
				constants.AnnotationIPFamily:    "IPv6Only",
				constants.AnnotationNetworkType: "overlay",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with DualStack family in pod annotations", func() {
			By("create pod with DualStack family specified in annotation")
			pod := simplePodRender(podName, node1Name)
			pod.Annotations = map[string]string{
				constants.AnnotationIPFamily:    "dualStack",
				constants.AnnotationNetworkType: "overlay",
			}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					networkingv1.SortIPInstancePointerSlice(ipInstances)

					v4IPInstance := ipInstances[0]
					g.Expect(v4IPInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(v4IPInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))

					v6IPInstance := ipInstances[1]
					g.Expect(v6IPInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(v6IPInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Config pod with DualStack family in namespace annotations", func() {
			By("update assigned ip family in namespace annotations")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					if len(ns.Annotations) == 0 {
						ns.Annotations = map[string]string{}
					}
					ns.Annotations[constants.AnnotationNetworkType] = "overlay"
					ns.Annotations[constants.AnnotationIPFamily] = "dualstack"
					return nil
				})).
				Error().
				NotTo(HaveOccurred())

			By("create pod with no labels or annotations")
			pod := simplePodRender(podName, node1Name)
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					networkingv1.SortIPInstancePointerSlice(ipInstances)

					v4IPInstance := ipInstances[0]
					g.Expect(v4IPInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(v4IPInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))

					v6IPInstance := ipInstances[1]
					g.Expect(v6IPInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(v6IPInstance.Spec.Address.Version).To(Equal(networkingv1.IPv6))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("clean assigned network type in namespace annotations")
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			Expect(controllerutil.CreateOrPatch(
				context.Background(),
				k8sClient,
				ns,
				func() error {
					ns.Annotations = map[string]string{}
					return nil
				})).
				Error().
				NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(),
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      podName,
					},
				},
				client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("clean up test ip instances")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

		})
	})

	Context("Network and IP-family retain for single stateful pod", func() {
		var podName string
		var ownerReference metav1.OwnerReference

		BeforeEach(func() {
			podName = fmt.Sprintf("pod-%d", rand.Intn(10))
			ownerReference = statefulOwnerReferenceRender()
		})

		It("Allocate and retain network and ip-family for single stateful pod", func() {
			By("create a stateful pod requiring DualStack addresses and specified overlay network")
			var ipInstanceIPv4Name string
			var ipInstanceIPv6Name string
			pod := simplePodRender(podName, node3Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: overlayNetworkName,
				constants.AnnotationIPFamily:         "DualStack",
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the first allocated DualStack addresses")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					// sort by ip family order
					networkingv1.SortIPInstancePointerSlice(ipInstances)

					// check IPv4 IPInstance
					ipInstanceIPv4 := ipInstances[0]
					ipInstanceIPv4Name = ipInstanceIPv4.Name
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.NodeName).To(Equal(node3Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstanceIPv4.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					// check IPv6 IPInstance
					ipInstanceIPv6 := ipInstances[1]
					ipInstanceIPv6Name = ipInstanceIPv6.Name
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.NodeName).To(Equal(node3Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx = *ipInstanceIPv6.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))

					// check MAC address
					g.Expect(ipInstanceIPv4.Spec.Address.MAC).To(Equal(ipInstanceIPv6.Spec.Address.MAC))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("check the allocated DualStack addresses are reserved")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					// sort by ip family order
					networkingv1.SortIPInstancePointerSlice(ipInstances)

					// check IPv4 IPInstance
					ipInstanceIPv4 := ipInstances[0]
					g.Expect(ipInstanceIPv4.Name).To(Equal(ipInstanceIPv4Name))
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstanceIPv4.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					// check IPv6 IPInstance
					ipInstanceIPv6 := ipInstances[1]
					g.Expect(ipInstanceIPv6.Name).To(Equal(ipInstanceIPv6Name))
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(BeEmpty())
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.NodeName).To(BeEmpty())
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx = *ipInstanceIPv6.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))

					// check MAC address
					g.Expect(ipInstanceIPv4.Spec.Address.MAC).To(Equal(ipInstanceIPv6.Spec.Address.MAC))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("make sure pod deleted")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("recreate the stateful pod without labels or annotations")
			pod = simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check the allocated DualStack addresses are retained and reused")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(2))

					// sort by ip family order
					networkingv1.SortIPInstancePointerSlice(ipInstances)

					// check IPv4 IPInstance
					ipInstanceIPv4 := ipInstances[0]
					g.Expect(ipInstanceIPv4.Name).To(Equal(ipInstanceIPv4Name))
					g.Expect(ipInstanceIPv4.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv4.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstanceIPv4.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv4.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstanceIPv4.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv4.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv4.Spec.Subnet).To(BeElementOf(overlayIPv4SubnetName))

					// check IPv6 IPInstance
					ipInstanceIPv6 := ipInstances[1]
					g.Expect(ipInstanceIPv6.Name).To(Equal(ipInstanceIPv6Name))
					g.Expect(ipInstanceIPv6.Spec.Address.Version).To(Equal(networkingv1.IPv6))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstanceIPv6.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstanceIPv6.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstanceIPv6.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx = *ipInstanceIPv6.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))

					g.Expect(ipInstanceIPv6.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstanceIPv6.Spec.Subnet).To(BeElementOf(overlayIPv6SubnetName))

					// check MAC address
					g.Expect(ipInstanceIPv4.Spec.Address.MAC).To(Equal(ipInstanceIPv6.Spec.Address.MAC))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("make sure test ip instances cleaned up")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})
	})

	Context("Assign ip-pool for stateful pods", func() {
		var podName string
		var ownerReference = statefulOwnerReferenceRender()
		var idx = 0
		var ipPool = []string{
			"100.10.0.150",
			"100.10.0.160",
			"100.10.0.170",
		}

		BeforeEach(func() {
			podName = fmt.Sprintf("pod-sts-%d", idx)
		})

		It("Create the first stateful pod with overlay network and ip pool", func() {
			By("create a stateful pod with special annotations")
			pod := simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: overlayNetworkName,
				constants.AnnotationIPPool:           strings.Join(ipPool, ","),
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-sts-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(overlayIPv4SubnetName))

					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Address.IP).To(Equal(ipPool[idx] + "/24"))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Create the second stateful pod with overlay network and ip pool", func() {
			By("create a stateful pod with special annotations")
			pod := simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: overlayNetworkName,
				constants.AnnotationIPPool:           strings.Join(ipPool, ","),
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-sts-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(overlayIPv4SubnetName))

					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Address.IP).To(Equal(ipPool[idx] + "/24"))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Create the third stateful pod with overlay network and ip pool", func() {
			By("create a stateful pod with special annotations")
			pod := simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationSpecifiedNetwork: overlayNetworkName,
				constants.AnnotationIPPool:           strings.Join(ipPool, ","),
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the allocated ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))

					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-sts-%d", idx)))

					g.Expect(ipInstance.Spec.Network).To(Equal(overlayNetworkName))
					g.Expect(ipInstance.Spec.Subnet).To(Equal(overlayIPv4SubnetName))

					g.Expect(ipInstance.Spec.Address.Version).To(Equal(networkingv1.IPv4))
					g.Expect(ipInstance.Spec.Address.IP).To(Equal(ipPool[idx] + "/24"))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("make sure test ip instances cleaned up")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("increase pod index")
			idx++
		})
	})

	Context("Specify MAC address pool for pod", func() {
		var podName string
		var ownerReference metav1.OwnerReference
		var podNameIdx int
		var specifiedMACPool = []string{
			"00:16:ea:ae:3c:40",
			"08:00:20:0a:8c:6d",
			"08:00:20:0a:0a:40",
		}

		BeforeEach(func() {
			podNameIdx = rand.Intn(len(specifiedMACPool))
			podName = fmt.Sprintf("pod-%d", podNameIdx)
			ownerReference = statefulOwnerReferenceRender()
		})

		It("Specify mac address pool for single stateful pod", func() {
			By("create a stateful pod with mac-pool annotation")
			var ipInstanceName string
			pod := simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			pod.Annotations = map[string]string{
				constants.AnnotationMACPool: strings.Join(specifiedMACPool, ","),
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the MAC address of pod ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					ipInstanceName = ipInstance.Name
					g.Expect(ipInstance.Spec.Address.MAC).To(Equal(specifiedMACPool[podNameIdx]))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))
					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("remove stateful pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())

			By("make sure pod is deleted")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("recreate the stateful pod")
			pod = simplePodRender(podName, node1Name)
			pod.OwnerReferences = []metav1.OwnerReference{ownerReference}
			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("check the MAC address of the retained ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Name).To(Equal(ipInstanceName))
					g.Expect(ipInstance.Spec.Address.MAC).To(Equal(specifiedMACPool[podNameIdx]))
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Binding.ReferredObject).To(Equal(networkingv1.ObjectMeta{
						Kind: ownerReference.Kind,
						Name: ownerReference.Name,
						UID:  ownerReference.UID,
					}))
					g.Expect(ipInstance.Spec.Binding.Stateful).NotTo(BeNil())
					g.Expect(ipInstance.Spec.Binding.Stateful.Index).NotTo(BeNil())

					idx := *ipInstance.Spec.Binding.Stateful.Index
					g.Expect(pod.Name).To(Equal(fmt.Sprintf("pod-%d", idx)))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("delete the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		It("Specify MAC address pool for single non-stateful pod", func() {
			By("create a non-stateful pod with mac-pool annotation")
			pod := simplePodRender(podName, node1Name)
			pod.Annotations = map[string]string{
				constants.AnnotationMACPool: strings.Join(specifiedMACPool, ","),
			}
			Expect(k8sClient.Create(context.Background(), pod)).Should(Succeed())

			By("check the MAC address of pod ip instance")
			Eventually(
				func(g Gomega) {
					ipInstances, err := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ipInstances).To(HaveLen(1))

					ipInstance := ipInstances[0]
					g.Expect(ipInstance.Spec.Binding.PodUID).To(Equal(pod.UID))
					g.Expect(ipInstance.Spec.Binding.PodName).To(Equal(pod.Name))
					g.Expect(ipInstance.Spec.Binding.NodeName).To(Equal(node1Name))
					g.Expect(ipInstance.Spec.Address.MAC).NotTo(Equal(specifiedMACPool[podNameIdx]))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("delete the test pod")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("make sure test ip instances cleaned up")
			Expect(k8sClient.DeleteAllOf(
				context.Background(),
				&networkingv1.IPInstance{},
				client.MatchingLabels{
					constants.LabelPod: transform.TransferPodNameForLabelValue(podName),
				},
				client.InNamespace("default"),
			)).NotTo(HaveOccurred())

			By("make sure test pod cleaned up")
			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: "default",
							Name:      podName,
						},
						&corev1.Pod{})
					g.Expect(err).NotTo(BeNil())
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

func simplePodRender(name string, node string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: node,
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "test",
				},
			},
		},
	}
}

func statefulOwnerReferenceRender() metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         "apps/v1",
		Kind:               "StatefulSet",
		Name:               "fake",
		UID:                uuid.NewUUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}
