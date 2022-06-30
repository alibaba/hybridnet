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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
)

var _ = Describe("IPInstance controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("IPInstance recycle by controller", func() {
		It("IPInstance should be released and unbind if being deleted", func() {
			By("record network usage available")
			networkUsage, err := ipamManager.GetNetworkUsage(underlayNetworkName)
			Expect(err).NotTo(HaveOccurred())
			Expect(networkUsage).NotTo(BeNil())
			Expect(networkUsage.GetByType(ipamtypes.IPv4)).NotTo(BeNil())

			availableOld := networkUsage.GetByType(ipamtypes.IPv4).Available

			By("create a pod for IP allocation")
			pod := simplePodRender("test-pod-for-ipinstance", node1Name)

			Expect(k8sClient.Create(context.Background(), pod)).NotTo(HaveOccurred())

			By("waiting IP allocation for pod")
			var ipInstanceName string
			Eventually(
				func() int {
					ips, _ := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					if len(ips) == 1 {
						ipInstanceName = ips[0].Name
					}
					return len(ips)
				}).
				WithTimeout(10 * time.Second).
				WithPolling(time.Second).
				Should(Equal(1))

			By("check network usage available")
			networkUsage, err = ipamManager.GetNetworkUsage(underlayNetworkName)
			Expect(err).NotTo(HaveOccurred())
			Expect(networkUsage).NotTo(BeNil())
			Expect(networkUsage.GetByType(ipamtypes.IPv4)).NotTo(BeNil())

			availableAfterAllocation := networkUsage.GetByType(ipamtypes.IPv4).Available
			Expect(availableAfterAllocation).To(Equal(availableOld - 1))

			By("deleting pod and IPInstance")
			Expect(k8sClient.Delete(context.Background(), pod, client.GracePeriodSeconds(0))).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(),
				&networkingv1.IPInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ipInstanceName,
						Namespace: pod.Namespace,
					},
				})).NotTo(HaveOccurred())

			By("check IPInstance release")
			Eventually(
				func() int {
					ips, _ := utils.ListAllocatedIPInstancesOfPod(context.Background(), k8sClient, pod)
					return len(ips)
				}).
				WithTimeout(10 * time.Second).
				WithPolling(time.Second).
				Should(Equal(0))

			Eventually(
				func(g Gomega) {
					err := k8sClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: pod.Namespace,
							Name:      ipInstanceName,
						},
						&networkingv1.IPInstance{})
					g.Expect(err).NotTo(BeNil())
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("check network usage available")
			networkUsage, err = ipamManager.GetNetworkUsage(underlayNetworkName)
			Expect(err).NotTo(HaveOccurred())
			Expect(networkUsage).NotTo(BeNil())
			Expect(networkUsage.GetByType(ipamtypes.IPv4)).NotTo(BeNil())

			availableAfterUnbind := networkUsage.GetByType(ipamtypes.IPv4).Available
			Expect(availableAfterUnbind).To(Equal(availableOld))
		})
	})

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
