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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	ipammanager "github.com/alibaba/hybridnet/pkg/ipam/manager"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
)

var _ = Describe("IPAM controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("Initialization check", func() {
		It("Check IPAM manager basic fields", func() {
			By("Cast interface to inner type")
			manager, ok := ipamManager.(*ipammanager.Manager)
			Expect(ok).To(BeTrue())

			By("Check all fields of IPAM manager")
			Expect(manager.NetworkGetter).NotTo(BeNil())
			Expect(manager.SubnetGetter).NotTo(BeNil())
			Expect(manager.IPSetGetter).NotTo(BeNil())
			Expect(manager.NetworkSet).NotTo(BeNil())
		})

		It("Check IPAM manager initialization", func() {
			By("Cast interface to inner type")
			manager, ok := ipamManager.(*ipammanager.Manager)
			Expect(ok).To(BeTrue())

			By("Waiting for IPAM manager initialized by controller")
			Eventually(
				func() int {
					manager.RLock()
					defer manager.RUnlock()
					return len(manager.NetworkSet)
				}()).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Equal(2))

			By("Check networks")
			Expect(manager.NetworkSet.ListNetworkToNames()).To(ConsistOf(underlayNetworkName, overlayNetworkName))

			Expect(manager.NetworkSet.CheckNetworkByType(underlayNetworkName, ipamtypes.Underlay)).To(BeTrue())
			Expect(manager.NetworkSet.CheckNetworkByType(overlayNetworkName, ipamtypes.Overlay)).To(BeTrue())

			By("Check underlay network")
			underlayNetwork, err := manager.NetworkSet.GetNetworkByName(underlayNetworkName)
			Expect(err).NotTo(HaveOccurred())
			Expect(underlayNetwork.IPv4Subnets).NotTo(BeNil())
			Expect(underlayNetwork.IPv4Subnets.Subnets).To(HaveLen(1))
			Expect(underlayNetwork.IPv4Subnets.Subnets[0].Name).To(Equal(underlaySubnetName))

			By("Check overlay network")
			overlayNetwork, err := manager.NetworkSet.GetNetworkByName(overlayNetworkName)
			Expect(err).NotTo(HaveOccurred())
			Expect(overlayNetwork.IPv4Subnets).NotTo(BeNil())
			Expect(overlayNetwork.IPv6Subnets).NotTo(BeNil())
			Expect(overlayNetwork.SubnetCount()).To(Equal(2))

			availableIPv4Subnet, err := overlayNetwork.GetIPv4SubnetByNameOrAvailable("")
			Expect(err).NotTo(HaveOccurred())
			Expect(availableIPv4Subnet).NotTo(BeNil())
			Expect(availableIPv4Subnet.Name).To(Equal(overlayIPv4SubnetName))

			availableIPv6Subnet, err := overlayNetwork.GetIPv6SubnetByNameOrAvailable("")
			Expect(err).NotTo(HaveOccurred())
			Expect(availableIPv6Subnet).NotTo(BeNil())
			Expect(availableIPv6Subnet.Name).To(Equal(overlayIPv6SubnetName))

			availableIPv4Subnet, availableIPv6Subnet, err = overlayNetwork.GetDualStackSubnetsByNameOrAvailable("", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(availableIPv4Subnet).NotTo(BeNil())
			Expect(availableIPv4Subnet.Name).To(Equal(overlayIPv4SubnetName))
			Expect(availableIPv6Subnet).NotTo(BeNil())
			Expect(availableIPv6Subnet.Name).To(Equal(overlayIPv6SubnetName))
		})
	})

	Context("Unlock", func() {
		testLock.Unlock()
	})
})
