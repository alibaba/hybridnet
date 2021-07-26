/*
Copyright 2021 The Rama Authors.

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

package controller

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	daemonutils "github.com/oecp/rama/pkg/daemon/utils"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

func (c *Controller) enqueueAddOrDeleteIPInstance(obj interface{}) {
	c.ipInstanceQueue.Add(ActionReconcileIPInstance)
}

func (c *Controller) enqueueUpdateIPInstance(oldObj, newObj interface{}) {
	oldIPInstance := oldObj.(*ramav1.IPInstance)
	newIPInstance := newObj.(*ramav1.IPInstance)

	if oldIPInstance.Status.NodeName != newIPInstance.Status.NodeName {
		c.ipInstanceQueue.Add(ActionReconcileIPInstance)
	}
}

func (c *Controller) processNextIPInstanceWorkItem() bool {
	obj, shutdown := c.ipInstanceQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.ipInstanceQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.ipInstanceQueue.Forget(obj)
			klog.Errorf("expected string in work queue but got %#v", obj)
			return nil
		}
		if err := c.reconcileIPInfo(); err != nil {
			c.ipInstanceQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.ipInstanceQueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}
	return true
}

func (c *Controller) runIPInstanceWorker() {
	for c.processNextIPInstanceWorkItem() {
	}
}

func (c *Controller) reconcileIPInfo() error {
	start := time.Now()
	defer func() {
		endTime := time.Since(start)
		klog.Infof("IPInstance information reconciled, took %v", endTime)
	}()

	networkList, err := c.networkLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list network failed: %v", err)
	}

	var overlayExist bool
	var overlayForwardNodeIfName string
	for _, network := range networkList {
		if ramav1.GetNetworkType(network) == ramav1.NetworkTypeOverlay {
			netID := network.Spec.NetID
			overlayForwardNodeIfName, err = containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, netID)
			if err != nil {
				return fmt.Errorf("generate vxlan forward node if name failed: %v", err)
			}
			overlayExist = true
			break
		}
	}

	if !c.upgradeWorkDone {
		if err := ensureExistPodConfigs(c.config.LocalDirectTableNum); err != nil {
			return fmt.Errorf("ensure exist pod config failed: %v", err)
		}
		c.upgradeWorkDone = true
	}

	nodeSelector := labels.SelectorFromSet(labels.Set{
		constants.LabelNode: c.config.NodeName,
	})

	ipInstances, err := c.ipInstanceLister.List(nodeSelector)
	if err != nil {
		return fmt.Errorf("list ip instances for node %v error: %v", c.config.NodeName, err)
	}

	c.neighV4Manager.ResetInfos()
	c.neighV6Manager.ResetInfos()

	c.addrV4Manager.ResetInfos()

	for _, ipInstance := range ipInstances {
		netID := ipInstance.Spec.Address.NetID
		if netID == nil {
			return fmt.Errorf("NetID of ip instance %v should not be nil", ipInstance.Name)
		}

		podIP, subnetCidr, err := net.ParseCIDR(ipInstance.Spec.Address.IP)
		if err != nil {
			return fmt.Errorf("parse pod ip %v error: %v", ipInstance.Spec.Address.IP, err)
		}

		network, err := c.networkLister.Get(ipInstance.Spec.Network)
		if err != nil {
			return fmt.Errorf("get network for ip instance %v failed: %v", ipInstance.Name, err)
		}

		var forwardNodeIfName string
		switch ramav1.GetNetworkType(network) {
		case ramav1.NetworkTypeUnderlay:
			forwardNodeIfName, err = containernetwork.GenerateVlanNetIfName(c.config.NodeVlanIfName, netID)
			if err != nil {
				return fmt.Errorf("generate vlan forward node interface name failed: %v", err)
			}

			if ipInstance.Spec.Address.Version == ramav1.IPv4 {
				c.addrV4Manager.TryAddPodInfo(forwardNodeIfName, subnetCidr, podIP)
			}

		case ramav1.NetworkTypeOverlay:
			forwardNodeIfName, err = containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, netID)
			if err != nil {
				return fmt.Errorf("generate vxlan forward node interface name failed: %v", err)
			}
		}

		// create proxy neigh
		neighManager := c.getNeighManager(ipInstance.Spec.Address.Version)

		if overlayExist {
			// Every underlay pod should also add a proxy neigh on overlay forward interface.
			// neighManager.AddPodInfo is idempotent
			neighManager.AddPodInfo(podIP, overlayForwardNodeIfName)
		}
		neighManager.AddPodInfo(podIP, forwardNodeIfName)
	}

	if err := c.neighV4Manager.SyncNeighs(); err != nil {
		return fmt.Errorf("sync ipv4 neighs failed: %v", err)
	}

	if err := c.neighV6Manager.SyncNeighs(); err != nil {
		return fmt.Errorf("sync ipv6 neighs failed: %v", err)
	}

	if err := c.addrV4Manager.SyncAddresses(c.getIPInstanceByAddress); err != nil {
		return fmt.Errorf("sync ipv4 addresses failed: %v", err)
	}

	return nil
}

// TODO: update logic, need to be removed further
func ensureExistPodConfigs(localDirectTableNum int) error {
	var netnsPaths []string
	var netnsDir string

	if daemonutils.ValidDockerNetnsDir(containernetwork.DockerNetnsDir) {
		netnsDir = containernetwork.DockerNetnsDir
	} else {
		klog.Infof("%s not exist, try %s", containernetwork.DockerNetnsDir, containernetwork.ContainerdNetnsDir)
		netnsDir = containernetwork.ContainerdNetnsDir
	}

	files, err := ioutil.ReadDir(netnsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, f := range files {
		if f.Name() == "default" {
			continue
		}
		fpath := netnsDir + "/" + f.Name()
		if daemonutils.IsProcFS(fpath) || daemonutils.IsNsFS(fpath) {
			netnsPaths = append(netnsPaths, fpath)
		}
	}

	klog.Infof("load exist netns: %v", netnsPaths)

	var hostLinkIndex int
	allocatedIPs := map[ramav1.IPVersion]*containernetwork.IPInfo{}

	for _, netns := range netnsPaths {
		nsHandler, err := ns.GetNS(netns)
		if err != nil {
			return fmt.Errorf("get ns error: %v", err)
		}

		err = nsHandler.Do(func(netNS ns.NetNS) error {
			link, err := netlink.LinkByName(containernetwork.ContainerNicName)
			if err != nil {
				return fmt.Errorf("get container interface error: %v", err)
			}

			v4Addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
			if err != nil {
				return fmt.Errorf("get v4 container interface addr failed: %v", err)
			}

			var v4GatewayIP net.IP
			if len(v4Addrs) == 0 {
				allocatedIPs[ramav1.IPv4] = nil
			} else {
				defaultRoute, err := containernetwork.GetDefaultRoute(netlink.FAMILY_V4)
				if err != nil {
					return fmt.Errorf("get ipv4 default route failed: %v", err)
				}
				v4GatewayIP = defaultRoute.Gw
			}

			for _, addr := range v4Addrs {
				allocatedIPs[ramav1.IPv4] = &containernetwork.IPInfo{
					Addr: addr.IP,
					Gw:   v4GatewayIP,
				}
			}

			v6Addrs, err := netlink.AddrList(link, netlink.FAMILY_V6)
			if err != nil {
				return fmt.Errorf("get v6 container interface addr failed: %v", err)
			}

			var v6GatewayIP net.IP
			if len(v6Addrs) == 0 {
				allocatedIPs[ramav1.IPv6] = nil
			} else {
				defaultRoute, err := containernetwork.GetDefaultRoute(netlink.FAMILY_V6)
				if err != nil {
					return fmt.Errorf("get ipv6 default route failed: %v", err)
				}
				v6GatewayIP = defaultRoute.Gw
			}

			for _, addr := range v6Addrs {
				allocatedIPs[ramav1.IPv6] = &containernetwork.IPInfo{
					Addr: addr.IP,
					Gw:   v6GatewayIP,
				}
			}

			_, hostLinkIndex, err = ip.GetVethPeerIfindex(containernetwork.ContainerNicName)
			if err != nil {
				return fmt.Errorf("get host link index error: %v", err)
			}

			return nil
		})

		if err != nil {
			klog.Errorf("get pod addresses and host link index error: %v", err)
		}

		hostLink, err := netlink.LinkByIndex(hostLinkIndex)
		if err != nil {
			return fmt.Errorf("get host link by index %v failed: %v", hostLinkIndex, err)
		}

		// this container doesn't belong to k8s
		if !strings.HasSuffix(hostLink.Attrs().Name, containernetwork.ContainerHostLinkSuffix) {
			continue
		}

		if hostLink.Attrs().MasterIndex != 0 {
			bridge, err := netlink.LinkByIndex(hostLink.Attrs().MasterIndex)
			if err != nil {
				return fmt.Errorf("get rama bridge by index %v failed: %v", hostLink.Attrs().MasterIndex, err)
			}

			if err := netlink.LinkDel(bridge); err != nil {
				return fmt.Errorf("delete rama bridge %v failed: %v", bridge.Attrs().Name, err)
			}
		}

		if err := containernetwork.ConfigureHostNic(hostLink.Attrs().Name, allocatedIPs, localDirectTableNum); err != nil {
			return fmt.Errorf("reconfigure host nic %v failed: %v", hostLink.Attrs().Name, err)
		}
	}

	return nil
}
