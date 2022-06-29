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
	"k8s.io/apimachinery/pkg/types"

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

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
