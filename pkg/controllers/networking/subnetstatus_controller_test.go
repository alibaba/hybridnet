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

					g.Expect(subnet.Status.Total).To(Equal(int32(253)))
					g.Expect(subnet.Status.Available).To(Equal(int32(253)))
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

					g.Expect(subnet.Status.Total).To(Equal(int32(254)))
					g.Expect(subnet.Status.Available).To(Equal(int32(254)))
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

					g.Expect(subnet.Status.Total).To(Equal(int32(255)))
					g.Expect(subnet.Status.Available).To(Equal(int32(255)))
					g.Expect(subnet.Status.Used).To(Equal(int32(0)))
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
