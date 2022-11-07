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

	"github.com/alibaba/hybridnet/pkg/ipam/manager"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
)

var _ = Describe("IPAM controller integration test suite", func() {
	Context("Lock", func() {
		testLock.Lock()
	})

	Context("Initialization check", func() {
		It("Check IPAM manager basic fields", func() {
			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Check all fields of IPAM manager")
			Expect(manager.NetworkGetter).NotTo(BeNil())
			Expect(manager.SubnetGetter).NotTo(BeNil())
			Expect(manager.IPSetGetter).NotTo(BeNil())
			Expect(manager.NetworkSet).NotTo(BeNil())
		})

		It("Check IPAM manager initialization", func() {
			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Waiting for IPAM manager initialized by controller")
			Eventually(
				func(g Gomega) {
					manager.RLock()
					defer manager.RUnlock()
					g.Expect(manager.NetworkSet).To(HaveLen(2))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("Check networks")
			Expect(manager.NetworkSet.ListNetworkToNames()).To(ConsistOf(underlayNetworkName, overlayNetworkName))

			Expect(manager.NetworkSet.CheckNetworkByType(underlayNetworkName, types.Underlay)).To(BeTrue())
			Expect(manager.NetworkSet.CheckNetworkByType(overlayNetworkName, types.Overlay)).To(BeTrue())

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

	Context("Refresh check after updating", func() {
		var testOverlayNetworkName = "ipam-overlay-network"
		var testOverlaySubnetName1 = "ipam-overlay-subnet1"
		var testOverlaySubnetName2 = "ipam-overlay-subnet2"
		var network = overlayNetworkRender(testOverlayNetworkName, 100)
		var subnet1 = subnetRender(testOverlaySubnetName1, testOverlayNetworkName, "192.167.56.0/24", nil, true)
		var subnet2 = subnetRender(testOverlaySubnetName2, testOverlayNetworkName, "fe90::0/120", nil, false)

		It("Check network refresh after creating one", func() {
			By("Create a new overlay network")
			Expect(k8sClient.Create(context.Background(), network)).NotTo(HaveOccurred())

			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Waiting for IPAM manager refreshed by controller")
			Eventually(
				func(g Gomega) {
					manager.RLock()
					defer manager.RUnlock()
					g.Expect(manager.NetworkSet).To(HaveLen(3))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())

			By("Check network names")
			Expect(manager.NetworkSet.ListNetworkToNames()).To(ContainElement(testOverlayNetworkName))

			By("Check created overlay network")
			createdOverlayNetwork, err := manager.NetworkSet.GetNetworkByName(testOverlayNetworkName)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdOverlayNetwork.IPv4Subnets).NotTo(BeNil())
			Expect(createdOverlayNetwork.IPv6Subnets).NotTo(BeNil())
			Expect(createdOverlayNetwork.Type).To(Equal(types.Overlay))
			Expect(createdOverlayNetwork.SubnetCount()).To(Equal(0))
		})

		It("Check subnet refresh after creating IPv4 one", func() {
			By("Create a new overlay IPv4 subnet")
			Expect(k8sClient.Create(context.Background(), subnet1)).NotTo(HaveOccurred())

			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Checking whether IPAM manager refreshed by controller correctly")
			Eventually(
				func(g Gomega) {
					manager.RLock()
					defer manager.RUnlock()
					g.Expect(manager.NetworkSet).To(HaveLen(3))

					parentNetwork, err := manager.NetworkSet.GetNetworkByName(testOverlayNetworkName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(parentNetwork).NotTo(BeNil())
					g.Expect(parentNetwork.SubnetCount()).To(Equal(1))
					g.Expect(parentNetwork.Type).To(Equal(types.Overlay))
					g.Expect(parentNetwork.IPv4Subnets.Subnets).To(HaveLen(1))

					createdSubnet, err := parentNetwork.GetSubnetByName(testOverlaySubnetName1)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(createdSubnet).NotTo(BeNil())
					g.Expect(createdSubnet.IsIPv4()).To(BeTrue())
					g.Expect(createdSubnet.IsAvailable()).To(BeTrue())
					g.Expect(createdSubnet.AvailableIPs.Count()).To(Equal(int(basicIPQuantity - networkAddress - broadcastAddress - gatewayAddress)))
					g.Expect(createdSubnet.UsingIPs.Count()).To(Equal(0))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Check subnet refresh after creating another IPv6 one", func() {
			By("Create a new overlay IPv6 subnet")
			Expect(k8sClient.Create(context.Background(), subnet2)).NotTo(HaveOccurred())

			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Checking whether IPAM manager refreshed by controller correctly")
			Eventually(
				func(g Gomega) {
					manager.RLock()
					defer manager.RUnlock()
					g.Expect(manager.NetworkSet).To(HaveLen(3))

					parentNetwork, err := manager.NetworkSet.GetNetworkByName(testOverlayNetworkName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(parentNetwork).NotTo(BeNil())
					g.Expect(parentNetwork.SubnetCount()).To(Equal(2))
					g.Expect(parentNetwork.Type).To(Equal(types.Overlay))
					g.Expect(parentNetwork.IPv4Subnets.Subnets).To(HaveLen(1))
					g.Expect(parentNetwork.IPv6Subnets.Subnets).To(HaveLen(1))

					createdSubnet, err := parentNetwork.GetSubnetByName(testOverlaySubnetName2)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(createdSubnet).NotTo(BeNil())
					g.Expect(createdSubnet.IsIPv6()).To(BeTrue())
					g.Expect(createdSubnet.IsAvailable()).To(BeTrue())
					g.Expect(createdSubnet.AvailableIPs.Count()).To(Equal(int(basicIPQuantity - networkAddress)))
					g.Expect(createdSubnet.UsingIPs.Count()).To(Equal(0))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Check subnet refresh after removing two", func() {
			By("Removing the two subnets")
			Expect(k8sClient.Delete(context.Background(), subnet1)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(context.Background(), subnet2)).NotTo(HaveOccurred())

			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Checking whether IPAM manager refreshed by controller correctly")
			Eventually(
				func(g Gomega) {
					manager.RLock()
					defer manager.RUnlock()
					g.Expect(manager.NetworkSet).To(HaveLen(3))

					parentNetwork, err := manager.NetworkSet.GetNetworkByName(testOverlayNetworkName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(parentNetwork).NotTo(BeNil())
					g.Expect(parentNetwork.SubnetCount()).To(Equal(0))
					g.Expect(parentNetwork.Type).To(Equal(types.Overlay))
					g.Expect(parentNetwork.IPv4Subnets).NotTo(BeNil())
					g.Expect(parentNetwork.IPv6Subnets).NotTo(BeNil())
					g.Expect(parentNetwork.IPv4Subnets.Subnets).To(HaveLen(0))
					g.Expect(parentNetwork.IPv6Subnets.Subnets).To(HaveLen(0))
				}).
				WithTimeout(30 * time.Second).
				WithPolling(time.Second).
				Should(Succeed())
		})

		It("Check network refresh after removing", func() {
			By("Removing the network")
			Expect(k8sClient.Delete(context.Background(), network)).NotTo(HaveOccurred())

			By("Cast interface to inner type")
			manager, ok := ipamManager.(*manager.Manager)
			Expect(ok).To(BeTrue())

			By("Checking whether IPAM manager refreshed by controller correctly")
			Eventually(
				func(g Gomega) {
					manager.RLock()
					defer manager.RUnlock()
					g.Expect(manager.NetworkSet).To(HaveLen(2))
					g.Expect(manager.NetworkSet.ListNetworkToNames()).NotTo(ContainElement(testOverlayNetworkName))
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
