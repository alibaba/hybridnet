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
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/vishvananda/netns"

	"github.com/oecp/rama/pkg/daemon/iptables"

	"github.com/heptiolabs/healthcheck"
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	ramainformer "github.com/oecp/rama/pkg/client/informers/externalversions"
	ramalister "github.com/oecp/rama/pkg/client/listers/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	daemonconfig "github.com/oecp/rama/pkg/daemon/config"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	"github.com/oecp/rama/pkg/daemon/neigh"
	"github.com/oecp/rama/pkg/daemon/route"
	"github.com/vishvananda/netlink"

	"golang.org/x/sys/unix"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const (
	ActionReconcileSubnet     = "ReconcileSubnet"
	ActionReconcileIPInstance = "ReconcileIPInstance"
	ActionReconcileNode       = "ReconcileNode"

	ByInstanceIPIndexer = "instanceIP"
)

// Controller is a set of kubernetes controllers
type Controller struct {
	subnetLister ramalister.SubnetLister
	subnetSynced cache.InformerSynced
	subnetQueue  workqueue.RateLimitingInterface

	ipInstanceLister  ramalister.IPInstanceLister
	ipInstanceSynced  cache.InformerSynced
	ipInstanceQueue   workqueue.RateLimitingInterface
	ipInstanceIndexer cache.Indexer

	networkLister ramalister.NetworkLister
	networkSynced cache.InformerSynced

	nodeLister corev1.NodeLister
	nodeSynced cache.InformerSynced
	nodeQueue  workqueue.RateLimitingInterface

	config *daemonconfig.Configuration

	routeV4Manager *route.Manager
	routeV6Manager *route.Manager

	neighV4Manager *neigh.Manager
	neighV6Manager *neigh.Manager

	iptablesV4Manager  *iptables.Manager
	iptablesV6Manager  *iptables.Manager
	iptablesSyncCh     chan struct{}
	iptablesSyncTicker *time.Ticker

	nodeIPCache *NodeIPCache

	upgradeWorkDone bool
}

// NewController returns a Controller to watch kubernetes CRD object events
func NewController(config *daemonconfig.Configuration,
	ramaInformerFactory ramainformer.SharedInformerFactory,
	kubeInformerFactory informers.SharedInformerFactory) (*Controller, error) {

	subnetInformer := ramaInformerFactory.Networking().V1().Subnets()
	networkInformer := ramaInformerFactory.Networking().V1().Networks()
	ipInstanceInformer := ramaInformerFactory.Networking().V1().IPInstances()
	nodeInformer := kubeInformerFactory.Core().V1().Nodes()

	routeV4Manager, err := route.CreateRouteManager(config.LocalDirectTableNum,
		config.ToOverlaySubnetTableNum,
		config.OverlayMarkTableNum,
		netlink.FAMILY_V4,
	)
	if err != nil {
		return nil, fmt.Errorf("create ipv4 route manager error: %v", err)
	}

	routeV6Manager, err := route.CreateRouteManager(config.LocalDirectTableNum,
		config.ToOverlaySubnetTableNum,
		config.OverlayMarkTableNum,
		netlink.FAMILY_V6,
	)
	if err != nil {
		return nil, fmt.Errorf("create ipv6 route manager error: %v", err)
	}

	neighV4Manager := neigh.CreateNeighManager(netlink.FAMILY_V4)
	neighV6Manager := neigh.CreateNeighManager(netlink.FAMILY_V6)

	iptablesV4Manager, err := iptables.CreateIPtablesManager(iptables.ProtocolIpv4)
	if err != nil {
		return nil, fmt.Errorf("create ipv4 iptables manager error: %v", err)
	}

	iptablesV6Manager, err := iptables.CreateIPtablesManager(iptables.ProtocolIpv6)
	if err != nil {
		return nil, fmt.Errorf("create ipv6 iptables manager error: %v", err)
	}

	if err := ipInstanceInformer.Informer().GetIndexer().AddIndexers(cache.Indexers{
		ByInstanceIPIndexer: indexByInstanceIP,
	}); err != nil {
		return nil, fmt.Errorf("add indexer to ip instance informer failed: %v", err)
	}

	controller := &Controller{
		subnetLister: subnetInformer.Lister(),
		subnetSynced: subnetInformer.Informer().HasSynced,
		subnetQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Subnet"),

		networkLister: networkInformer.Lister(),
		networkSynced: networkInformer.Informer().HasSynced,

		ipInstanceLister:  ipInstanceInformer.Lister(),
		ipInstanceSynced:  ipInstanceInformer.Informer().HasSynced,
		ipInstanceQueue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "IPInstance"),
		ipInstanceIndexer: ipInstanceInformer.Informer().GetIndexer(),

		nodeLister: nodeInformer.Lister(),
		nodeSynced: nodeInformer.Informer().HasSynced,
		nodeQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Node"),

		config: config,

		routeV4Manager: routeV4Manager,
		routeV6Manager: routeV6Manager,

		neighV4Manager: neighV4Manager,
		neighV6Manager: neighV6Manager,

		iptablesV4Manager:  iptablesV4Manager,
		iptablesV6Manager:  iptablesV6Manager,
		iptablesSyncCh:     make(chan struct{}, 1),
		iptablesSyncTicker: time.NewTicker(config.IptablesCheckDuration),

		nodeIPCache: NewNodeIPCache(),
	}

	_, err = config.KubeClient.CoreV1().Nodes().Get(context.TODO(), config.NodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s info %v", config.NodeName, err)
	}

	networkInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueAddOrDeleteNetwork,
		DeleteFunc: controller.enqueueAddOrDeleteNetwork,
		UpdateFunc: controller.enqueueUpdateNetwork,
	})

	subnetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueAddOrDeleteSubnet,
		DeleteFunc: controller.enqueueAddOrDeleteSubnet,
		UpdateFunc: controller.enqueueUpdateSubnet,
	})

	ipInstanceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueAddOrDeleteIPInstance,
		UpdateFunc: controller.enqueueUpdateIPInstance,
		DeleteFunc: controller.enqueueAddOrDeleteIPInstance,
	})

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueAddOrDeleteNode,
		UpdateFunc: controller.enqueueUpdateNode,
		DeleteFunc: controller.enqueueAddOrDeleteNode,
	})

	return controller, nil
}

// Run a Controller
func (c *Controller) Run(stopCh <-chan struct{}) error {
	klog.Info("Started controller")
	c.runHealthyServer()

	defer c.subnetQueue.ShutDown()
	defer c.ipInstanceQueue.ShutDown()
	defer c.nodeQueue.ShutDown()

	if ok := cache.WaitForCacheSync(stopCh, c.subnetSynced, c.ipInstanceSynced, c.networkSynced, c.nodeSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	go wait.Until(c.runSubnetWorker, time.Second, stopCh)
	go wait.Until(c.runIPInstanceWorker, time.Second, stopCh)
	go wait.Until(c.runNodeWorker, time.Second, stopCh)

	if err := c.handleLocalNetworkDeviceEvent(); err != nil {
		return fmt.Errorf("failed to handle local network device event: %v", err)
	}

	if err := c.handleVxlanInterfaceNeighEvent(); err != nil {
		return fmt.Errorf("failed to handle vxlan interface neigh event: %v", err)
	}

	c.iptablesSyncLoop()

	<-stopCh
	klog.Info("Shutting down controller")

	return nil
}

func (c *Controller) GetIPInstanceLister() ramalister.IPInstanceLister {
	return c.ipInstanceLister
}

func (c *Controller) GetNetworkLister() ramalister.NetworkLister {
	return c.networkLister
}

func (c *Controller) GetIPInstanceSynced() cache.InformerSynced {
	return c.ipInstanceSynced
}

// Once node network interface is set down to up for some reasons, the routes and neigh caches for this interface
// will be cleaned, which should cause unrecoverable problems. Listening "UP" netlink events for interfaces and
// triggering subnet and ip instance reconcile loop will be the best way to recover routes and neigh caches.
//
// Restart of vxlan interface will also trigger subnet and ip instance reconcile loop.
func (c *Controller) handleLocalNetworkDeviceEvent() error {
	linkCh := make(chan netlink.LinkUpdate)
	if err := netlink.LinkSubscribe(linkCh, nil); err != nil {
		return fmt.Errorf("subscribe link update event failed %v", err)
	}

	go func() {
		for {
			update := <-linkCh
			if (update.IfInfomsg.Flags&unix.IFF_UP != 0) &&
				!strings.HasSuffix(update.Link.Attrs().Name, containernetwork.ContainerHostLinkSuffix) &&
				!strings.HasSuffix(update.Link.Attrs().Name, containernetwork.ContainerInitLinkSuffix) &&
				!strings.HasPrefix(update.Link.Attrs().Name, "veth") {

				// Create event to flush routes and neigh caches.
				c.subnetQueue.Add(ActionReconcileSubnet)
				c.ipInstanceQueue.Add(ActionReconcileIPInstance)
			}
		}
	}()

	addrCh := make(chan netlink.AddrUpdate)
	if err := netlink.AddrSubscribe(addrCh, nil); err != nil {
		return fmt.Errorf("subscribe address update event failed %v", err)
	}

	go func() {
		for {
			update := <-addrCh
			if containernetwork.CheckIPIsGlobalUnicast(update.LinkAddress.IP) {
				// Create event to update node configuration.
				c.nodeQueue.Add(ActionReconcileNode)
			}
		}
	}()

	return nil
}

func (c *Controller) handleVxlanInterfaceNeighEvent() error {

	ipSearch := func(ip net.IP, link netlink.Link) error {
		var vtepMac net.HardwareAddr

		// Try to find node ip.
		if mac, exist := c.nodeIPCache.SearchIP(ip); exist {
			vtepMac = mac
		} else {
			ipInstanceList, err := c.ipInstanceIndexer.ByIndex(ByInstanceIPIndexer, ip.String())
			if err != nil {
				return fmt.Errorf("get ip instance by ip %v indexer failed: %v", ip.String(), err)
			}

			if len(ipInstanceList) > 1 {
				return fmt.Errorf("get more than one ip instance for ip %v", ip.String())
			}

			if len(ipInstanceList) == 1 {
				instance, ok := ipInstanceList[0].(*ramav1.IPInstance)
				if !ok {
					return fmt.Errorf("transform obj to ipinstance failed")
				}

				nodeName := instance.Labels[constants.LabelNode]

				node, err := c.nodeLister.Get(nodeName)
				if err != nil {
					return fmt.Errorf("get node %v failed: %v", nodeName, err)
				}

				vtepMac, err = net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
				if err != nil {
					return fmt.Errorf("parse vtep mac %v failed: %v",
						node.Annotations[constants.AnnotationNodeVtepMac], err)
				}
			}
		}

		if len(vtepMac) == 0 {
			return fmt.Errorf("no matched node or pod ip cache found for ip %v", ip.String())
		}

		neighEntry := netlink.Neigh{
			LinkIndex:    link.Attrs().Index,
			State:        netlink.NUD_REACHABLE,
			Type:         syscall.RTN_UNICAST,
			IP:           ip,
			HardwareAddr: vtepMac,
		}
		if err := netlink.NeighSet(&neighEntry); err != nil {
			return fmt.Errorf("set neigh %v failed: %v", neighEntry.String(), err)
		}

		klog.V(4).Infof("resolve ip %v from user space success, target vtep mac %v", ip.String(), vtepMac.String())

		return nil
	}

	ipSearchExecWrapper := func(ip net.IP, link netlink.Link) {
		if err := ipSearch(ip, link); err != nil {
			klog.Errorf("overlay proxy neigh failed: %v", err)
		}
	}

	ch := make(chan netlink.NeighUpdate)

	hostNetNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("get root netns failed: %v", err)
	}

	if err := netlink.NeighSubscribeAt(hostNetNs, ch, nil); err != nil {
		return fmt.Errorf("subscribe neigh update event failed: %v", err)
	}

	go func() {
		for {
			update := <-ch
			if isNeighResolving(update.State) {
				if update.Type == syscall.RTM_DELNEIGH {
					continue
				}

				link, err := netlink.LinkByIndex(update.LinkIndex)
				if err != nil {
					klog.Errorf("get link by index %v failed: %v", update.LinkIndex, err)
					continue
				}

				if strings.Contains(link.Attrs().Name, containernetwork.VxlanLinkInfix) {
					go ipSearchExecWrapper(update.IP, link)
				}
			}
		}
	}()

	return nil
}

func (c *Controller) iptablesSyncLoop() {
	iptablesSyncFunc := func() error {
		c.iptablesV4Manager.Reset()
		c.iptablesV6Manager.Reset()

		networkList, err := c.networkLister.List(labels.Everything())
		if err != nil {
			return fmt.Errorf("list network failed: %v", err)
		}

		var overlayExist bool
		for _, network := range networkList {
			if ramav1.GetNetworkType(network) == ramav1.NetworkTypeOverlay {
				netID := network.Spec.NetID

				overlayIfName, err := containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, netID)
				if err != nil {
					return fmt.Errorf("generate vxlan forward node if name failed: %v", err)
				}

				c.iptablesV4Manager.SetOverlayIfName(overlayIfName)
				c.iptablesV6Manager.SetOverlayIfName(overlayIfName)

				overlayExist = true
				break
			}
		}

		// Record node ips.
		nodeList, err := c.nodeLister.List(labels.Everything())
		if err != nil {
			return fmt.Errorf("list node failed: %v", err)
		}

		for _, node := range nodeList {
			if node.Annotations[constants.AnnotationNodeVtepMac] == "" ||
				node.Annotations[constants.AnnotationNodeVtepIP] == "" ||
				node.Annotations[constants.AnnotationNodeIPList] == "" {
				klog.Infof("node %v's vtep information has not been updated", node.Name)
				continue
			}

			ipStringList := strings.Split(node.Annotations[constants.AnnotationNodeIPList], ",")
			for _, ipString := range ipStringList {
				ip := net.ParseIP(ipString)
				if ip.To4() != nil {
					// v4 address
					c.iptablesV4Manager.RecordNodeIP(ip)
				} else {
					// v6 address
					c.iptablesV6Manager.RecordNodeIP(ip)
				}
			}
		}

		// Record subnet cidr.
		subnetList, err := c.subnetLister.List(labels.Everything())
		if err != nil {
			return fmt.Errorf("list subnet failed: %v", err)
		}

		for _, subnet := range subnetList {
			_, cidr, err := net.ParseCIDR(subnet.Spec.Range.CIDR)
			if err != nil {
				return fmt.Errorf("parse subnet cidr %v failed: %v", subnet.Spec.Range.CIDR, err)
			}

			network, err := c.networkLister.Get(subnet.Spec.Network)
			if err != nil {
				return fmt.Errorf("failed to get network for subnet %v", subnet.Name)
			}

			iptablesManager := c.getIPtablesManager(subnet.Spec.Range.Version)
			iptablesManager.RecordSubnet(cidr, ramav1.GetNetworkType(network) == ramav1.NetworkTypeOverlay)
		}

		// Sync rules.
		if overlayExist {
			if err := c.iptablesV4Manager.SyncRules(); err != nil {
				return fmt.Errorf("sync v4 iptables rule failed: %v", err)
			}

			if err := c.iptablesV6Manager.SyncRules(); err != nil {
				return fmt.Errorf("sync v6 iptables rule failed: %v", err)
			}
		}

		return nil
	}

	go func() {
		for {
			select {
			case <-c.iptablesSyncCh:
				if err := iptablesSyncFunc(); err != nil {
					klog.Errorf("sync iptables rule failed: %v", err)
				}
			case <-c.iptablesSyncTicker.C:
				c.iptablesSyncTrigger()
			}
		}
	}()
}

func (c *Controller) iptablesSyncTrigger() {
	select {
	case c.iptablesSyncCh <- struct{}{}:
	default:
	}
}

func (c *Controller) runHealthyServer() {
	health := healthcheck.NewHandler()

	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", c.config.BindPort), health)
	}()

	klog.Infof("Listen on port: %d", c.config.BindPort)
}

func isNeighResolving(state int) bool {
	return (state & (netlink.NUD_INCOMPLETE | netlink.NUD_STALE | netlink.NUD_DELAY | netlink.NUD_PROBE)) != 0
}

func indexByInstanceIP(obj interface{}) ([]string, error) {
	instance, ok := obj.(*ramav1.IPInstance)
	if ok {
		podIP, _, err := net.ParseCIDR(instance.Spec.Address.IP)
		if err != nil {
			return []string{}, fmt.Errorf("parse pod ip %v failed: %v", instance.Spec.Address.IP, err)
		}

		return []string{podIP.String()}, nil
	}
	return []string{}, nil
}
