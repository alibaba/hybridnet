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

package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/hybridnet/pkg/daemon/bgp"
	"github.com/go-logr/logr"
	"github.com/heptiolabs/healthcheck"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/addr"
	daemonconfig "github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
	"github.com/alibaba/hybridnet/pkg/daemon/iptables"
	"github.com/alibaba/hybridnet/pkg/daemon/neigh"
	"github.com/alibaba/hybridnet/pkg/daemon/route"
	"github.com/alibaba/hybridnet/pkg/feature"
)

const (
	ActionReconcileSubnet     = "AllSubnetsRelatedToThisNode"
	ActionReconcileIPInstance = "AllIPInstancesRelatedToThisNode"
	ActionReconcileNode       = "AllNodes"

	InstanceIPIndex = "instanceIP"
	EndpointIPIndex = "endpointIP"

	NeighUpdateChanSize = 2000
	LinkUpdateChainSize = 200
	AddrUpdateChainSize = 200

	NetlinkSubscribeRetryInterval = 10 * time.Second
)

type CtrlHub struct {
	config *daemonconfig.Configuration
	mgr    ctrl.Manager

	subnetControllerTriggerSource     *simpleTriggerSource
	ipInstanceControllerTriggerSource *simpleTriggerSource
	nodeControllerTriggerSource       *simpleTriggerSource

	routeV4Manager *route.Manager
	routeV6Manager *route.Manager

	neighV4Manager *neigh.Manager
	neighV6Manager *neigh.Manager

	addrV4Manager *addr.Manager

	bgpManager *bgp.Manager

	iptablesV4Manager  *iptables.Manager
	iptablesV6Manager  *iptables.Manager
	iptablesSyncCh     chan struct{}
	iptablesSyncTicker *time.Ticker

	nodeIPCache *NodeIPCache

	upgradeWorkDone bool

	logger logr.Logger
}

func NewCtrlHub(config *daemonconfig.Configuration, mgr ctrl.Manager, logger logr.Logger) (*CtrlHub, error) {
	routeV4Manager, err := route.CreateRouteManager(config.LocalDirectTableNum,
		config.ToOverlaySubnetTableNum,
		config.OverlayMarkTableNum,
		netlink.FAMILY_V4,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ipv4 route manager: %v", err)
	}

	routeV6Manager, err := route.CreateRouteManager(config.LocalDirectTableNum,
		config.ToOverlaySubnetTableNum,
		config.OverlayMarkTableNum,
		netlink.FAMILY_V6,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ipv6 route manager: %v", err)
	}

	neighV4Manager := neigh.CreateNeighManager(netlink.FAMILY_V4)
	neighV6Manager := neigh.CreateNeighManager(netlink.FAMILY_V6)

	iptablesV4Manager, err := iptables.CreateIPtablesManager(iptables.ProtocolIpv4)
	if err != nil {
		return nil, fmt.Errorf("failed to create ipv4 iptables manager: %v", err)
	}

	iptablesV6Manager, err := iptables.CreateIPtablesManager(iptables.ProtocolIpv6)
	if err != nil {
		return nil, fmt.Errorf("failed to create ipv6 iptables manager: %v", err)
	}

	addrV4Manager := addr.CreateAddrManager(netlink.FAMILY_V4, config.NodeName)

	bgpManager, err := bgp.NewManager(config.NodeBGPIfName, config.BGPgRPCServerAddress, logger.WithName("bgp-server"))
	if err != nil {
		return nil, fmt.Errorf("failed to create bgp manager: %v", err)
	}

	ctrlHub := &CtrlHub{
		config: config,
		mgr:    mgr,

		subnetControllerTriggerSource:     &simpleTriggerSource{key: ActionReconcileSubnet},
		ipInstanceControllerTriggerSource: &simpleTriggerSource{key: ActionReconcileIPInstance},
		nodeControllerTriggerSource:       &simpleTriggerSource{key: ActionReconcileNode},

		routeV4Manager: routeV4Manager,
		routeV6Manager: routeV6Manager,

		neighV4Manager: neighV4Manager,
		neighV6Manager: neighV6Manager,

		addrV4Manager: addrV4Manager,

		bgpManager: bgpManager,

		iptablesV4Manager:  iptablesV4Manager,
		iptablesV6Manager:  iptablesV6Manager,
		iptablesSyncCh:     make(chan struct{}, 1),
		iptablesSyncTicker: time.NewTicker(config.IptablesCheckDuration),

		nodeIPCache: NewNodeIPCache(),

		logger: logger,
	}

	thisNode := &corev1.Node{}
	if err = mgr.GetAPIReader().Get(context.TODO(), types.NamespacedName{Name: config.NodeName}, thisNode); err != nil {
		return nil, fmt.Errorf("failed to get node %s info %v", config.NodeName, err)
	}

	return ctrlHub, nil
}

func (c *CtrlHub) Run(ctx context.Context) error {
	c.runHealthyServer()

	if err := c.mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.IPInstance{},
		InstanceIPIndex, instanceIPIndexer); err != nil {
		return fmt.Errorf("failed to add instance ip indexer to manager: %v", err)
	}

	if feature.MultiClusterEnabled() {
		if err := c.mgr.GetFieldIndexer().IndexField(context.TODO(), &multiclusterv1.RemoteVtep{},
			EndpointIPIndex, endpointIPIndexer); err != nil {
			return fmt.Errorf("failed to add endpoint ip indexer to manager: %v", err)
		}
	}

	if err := c.setupSubnetController(); err != nil {
		return fmt.Errorf("failed to setup subnet controller: %v", err)
	}

	if err := c.setupIPInstanceController(); err != nil {
		return fmt.Errorf("failed to setup ip instance controller: %v", err)
	}

	if err := c.setupNodeController(); err != nil {
		return fmt.Errorf("failed to setup node controller: %v", err)
	}

	if err := c.handleLocalNetworkDeviceEvent(); err != nil {
		return fmt.Errorf("failed to handle local network device event: %v", err)
	}

	if err := c.handleVxlanInterfaceNeighEvent(); err != nil {
		return fmt.Errorf("failed to handle vxlan interface neigh event: %v", err)
	}

	c.iptablesSyncLoop()

	if err := c.mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start controller manager: %v", err)
	}
	return nil
}

func (c *CtrlHub) CacheSynced(ctx context.Context) bool {
	return c.mgr.GetCache().WaitForCacheSync(ctx)
}

func (c *CtrlHub) GetMgrClient() client.Client {
	return c.mgr.GetClient()
}

func (c *CtrlHub) GetMgrAPIReader() client.Reader {
	return c.mgr.GetAPIReader()
}

func (c *CtrlHub) setupSubnetController() error {
	subnetController, err := controller.New("subnet", c.mgr, controller.Options{
		Reconciler: &subnetReconciler{
			Client:     c.mgr.GetClient(),
			ctrlHubRef: c,
		}})
	if err != nil {
		return fmt.Errorf("failed to create subnet controller: %v", err)
	}

	if err := subnetController.Watch(&source.Kind{Type: &networkingv1.Subnet{}},
		&fixedKeyHandler{key: ActionReconcileSubnet},
		&predicate.ResourceVersionChangedPredicate{},
		&predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldSubnet := updateEvent.ObjectOld.(*networkingv1.Subnet)
				newSubnet := updateEvent.ObjectNew.(*networkingv1.Subnet)

				oldSubnetNetID := oldSubnet.Spec.NetID
				newSubnetNetID := newSubnet.Spec.NetID

				if (oldSubnetNetID == nil && newSubnetNetID != nil) ||
					(oldSubnetNetID != nil && newSubnetNetID == nil) ||
					(oldSubnetNetID != nil && newSubnetNetID != nil && *oldSubnetNetID != *newSubnetNetID) ||
					oldSubnet.Spec.Network != newSubnet.Spec.Network ||
					!reflect.DeepEqual(oldSubnet.Spec.Range, newSubnet.Spec.Range) ||
					networkingv1.IsSubnetAutoNatOutgoing(&oldSubnet.Spec) != networkingv1.IsSubnetAutoNatOutgoing(&newSubnet.Spec) {
					return true
				}
				return false
			},
		}); err != nil {
		return fmt.Errorf("failed to watch networkingv1.Subnet for subnet controller: %v", err)
	}

	if err := subnetController.Watch(&source.Kind{Type: &networkingv1.Network{}},
		&fixedKeyHandler{key: ActionReconcileSubnet},
		&predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldNetwork := updateEvent.ObjectOld.(*networkingv1.Network)
				newNetwork := updateEvent.ObjectNew.(*networkingv1.Network)

				if len(oldNetwork.Status.SubnetList) != len(newNetwork.Status.SubnetList) ||
					len(oldNetwork.Status.NodeList) != len(newNetwork.Status.NodeList) {
					return true
				}

				for index, subnet := range oldNetwork.Status.SubnetList {
					if subnet != newNetwork.Status.SubnetList[index] {
						return true
					}
				}

				for index, node := range oldNetwork.Status.NodeList {
					if node != newNetwork.Status.NodeList[index] {
						return true
					}
				}
				return false
			},
		},
	); err != nil {
		return fmt.Errorf("failed to watch networkingv1.Network for subnet controller: %v", err)
	}

	if err := subnetController.Watch(&source.Kind{Type: &corev1.Node{}},
		&fixedKeyHandler{key: ActionReconcileSubnet},
		predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				if checkNodeUpdate(updateEvent) {
					return true
				}
				return false
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		}); err != nil {
		return fmt.Errorf("failed to watch corev1.Node for subnet controller: %v", err)
	}

	if err := subnetController.Watch(c.subnetControllerTriggerSource, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch subnetControllerTriggerSource for subnet controller: %v", err)
	}

	// enable multicluster feature
	if feature.MultiClusterEnabled() {
		if err := subnetController.Watch(&source.Kind{
			Type: &multiclusterv1.RemoteSubnet{}},
			&fixedKeyHandler{key: ActionReconcileSubnet},
			predicate.Funcs{
				UpdateFunc: func(updateEvent event.UpdateEvent) bool {
					oldRs := updateEvent.ObjectOld.(*multiclusterv1.RemoteSubnet)
					newRs := updateEvent.ObjectNew.(*multiclusterv1.RemoteSubnet)

					if oldRs.Spec.ClusterName != newRs.Spec.ClusterName ||
						!reflect.DeepEqual(oldRs.Spec.Range, newRs.Spec.Range) ||
						multiclusterv1.GetRemoteSubnetType(oldRs) != multiclusterv1.GetRemoteSubnetType(newRs) {
						return true
					}
					return false
				},
			},
		); err != nil {
			return fmt.Errorf("failed to watch multiclusterv1.RemoteSubnet for subnet controller: %v", err)
		}
	}

	return nil
}

func (c *CtrlHub) setupIPInstanceController() error {
	ipInstanceController, err := controller.New("ip-instance", c.mgr, controller.Options{
		Reconciler: &ipInstanceReconciler{
			Client:     c.mgr.GetClient(),
			ctrlHubRef: c,
		}})
	if err != nil {
		return fmt.Errorf("failed to create ip instance controller: %v", err)
	}

	if err := ipInstanceController.Watch(&source.Kind{Type: &networkingv1.IPInstance{}},
		&fixedKeyHandler{key: ActionReconcileIPInstance},
		&predicate.ResourceVersionChangedPredicate{},
		&predicate.LabelChangedPredicate{},
		&predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				ipInstance := createEvent.Object.(*networkingv1.IPInstance)
				return ipInstance.GetLabels()[constants.LabelNode] == c.config.NodeName
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				ipInstance := deleteEvent.Object.(*networkingv1.IPInstance)
				return ipInstance.GetLabels()[constants.LabelNode] == c.config.NodeName
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				oldIPInstance := updateEvent.ObjectOld.(*networkingv1.IPInstance)
				newIPInstance := updateEvent.ObjectNew.(*networkingv1.IPInstance)

				if newIPInstance.GetLabels()[constants.LabelNode] != c.config.NodeName ||
					oldIPInstance.GetLabels()[constants.LabelNode] != c.config.NodeName {
					return false
				}

				if oldIPInstance.Labels[constants.LabelNode] != newIPInstance.Labels[constants.LabelNode] {
					return true
				}
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				ipInstance := genericEvent.Object.(*networkingv1.IPInstance)
				return ipInstance.GetLabels()[constants.LabelNode] == c.config.NodeName
			},
		}); err != nil {
		return fmt.Errorf("failed to watch networkingv1.IPInstance for ip instance controller: %v", err)
	}

	if err := ipInstanceController.Watch(c.ipInstanceControllerTriggerSource, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch ipInstanceControllerTriggerSource for ip instance controller: %v", err)
	}

	return nil
}

func (c *CtrlHub) setupNodeController() error {
	nodeController, err := controller.New("node", c.mgr, controller.Options{
		Reconciler: &nodeReconciler{
			Client:     c.mgr.GetClient(),
			ctrlHubRef: c,
		}})
	if err != nil {
		return fmt.Errorf("failed to create node controller: %v", err)
	}

	if err := nodeController.Watch(&source.Kind{Type: &corev1.Node{}},
		&fixedKeyHandler{key: ActionReconcileNode},
		&predicate.ResourceVersionChangedPredicate{},
		&predicate.LabelChangedPredicate{},
		&predicate.Funcs{
			UpdateFunc: checkNodeUpdate,
		},
	); err != nil {
		return fmt.Errorf("failed to watch corev1.Node for node controller: %v", err)
	}

	if err := nodeController.Watch(&source.Kind{Type: &networkingv1.Network{}},
		&fixedKeyHandler{key: ActionReconcileNode},
		predicate.Funcs{
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return false
			},
			CreateFunc: func(createEvent event.CreateEvent) bool {
				network := createEvent.Object.(*networkingv1.Network)
				if networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay {
					return true
				}
				return false
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				network := deleteEvent.Object.(*networkingv1.Network)
				if networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay {
					return true
				}
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		},
	); err != nil {
		return fmt.Errorf("failed to watch networkingv1.Network for node controller: %v", err)
	}

	if err := nodeController.Watch(c.nodeControllerTriggerSource, &handler.Funcs{}); err != nil {
		return fmt.Errorf("failed to watch nodeControllerTriggerSource for node controller: %v", err)
	}

	if feature.MultiClusterEnabled() {
		if err := nodeController.Watch(&source.Kind{Type: &multiclusterv1.RemoteVtep{}},
			&fixedKeyHandler{key: ActionReconcileNode},
			predicate.Funcs{
				UpdateFunc: func(updateEvent event.UpdateEvent) bool {
					oldRemoteVtep := updateEvent.ObjectOld.(*multiclusterv1.RemoteVtep)
					newRemoteVtep := updateEvent.ObjectNew.(*multiclusterv1.RemoteVtep)

					if oldRemoteVtep.Spec.VTEPInfo.IP != newRemoteVtep.Spec.VTEPInfo.IP ||
						oldRemoteVtep.Spec.VTEPInfo.MAC != newRemoteVtep.Spec.VTEPInfo.MAC ||
						oldRemoteVtep.Annotations[constants.AnnotationNodeLocalVxlanIPList] != newRemoteVtep.Annotations[constants.AnnotationNodeLocalVxlanIPList] ||
						!isIPListEqual(oldRemoteVtep.Spec.EndpointIPList, newRemoteVtep.Spec.EndpointIPList) {
						return true
					}
					return false
				},
			},
		); err != nil {
			return fmt.Errorf("failed to watch multiclusterv1.RemoteVtep for node controller: %v", err)
		}
	}

	return nil
}

// Once node network interface is set from down to up for some reasons, the routes and neigh caches for this interface
// will be cleaned, which should cause unrecoverable problems. Listening "UP" netlink events for interfaces and
// triggering subnet and ip instance reconcile loop will be the best way to recover routes and neigh caches.
//
// Restart of vxlan interface will also trigger subnet and ip instance reconcile loop.
func (c *CtrlHub) handleLocalNetworkDeviceEvent() error {
	hostNetNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get root netns: %v", err)
	}

	go func() {
		for {
			linkCh := make(chan netlink.LinkUpdate, LinkUpdateChainSize)
			exitCh := make(chan struct{})

			errorCallback := func(err error) {
				c.logger.Error(err, "subscribe netlink link event exit with error")
				close(exitCh)
			}

			if err := netlink.LinkSubscribeWithOptions(linkCh, nil, netlink.LinkSubscribeOptions{
				Namespace:     &hostNetNs,
				ErrorCallback: errorCallback,
			}); err != nil {
				c.logger.Error(err, "failed to subscribe link update event")
				time.Sleep(NetlinkSubscribeRetryInterval)
				continue
			}

		linkLoop:
			for {
				select {
				case update := <-linkCh:
					if (update.IfInfomsg.Flags&unix.IFF_UP != 0) &&
						!containernetwork.CheckIfContainerNetworkLink(update.Link.Attrs().Name) {

						// Create event to flush routes and neigh caches.
						c.subnetControllerTriggerSource.Trigger()
						c.ipInstanceControllerTriggerSource.Trigger()
					}
				case <-exitCh:
					break linkLoop
				}
			}
		}
	}()

	go func() {
		for {
			addrCh := make(chan netlink.AddrUpdate, AddrUpdateChainSize)
			exitCh := make(chan struct{})

			errorCallback := func(err error) {
				c.logger.Error(err, "subscribe netlink addr event exit with error")
				close(exitCh)
			}

			if err := netlink.AddrSubscribeWithOptions(addrCh, nil, netlink.AddrSubscribeOptions{
				Namespace:     &hostNetNs,
				ErrorCallback: errorCallback,
			}); err != nil {
				c.logger.Error(err, "failed to subscribe address update event")
				time.Sleep(NetlinkSubscribeRetryInterval)
				continue
			}

		addrLoop:
			for {
				select {
				case update := <-addrCh:
					if containernetwork.CheckIPIsGlobalUnicast(update.LinkAddress.IP) {
						// Create event to update node configuration.
						c.nodeControllerTriggerSource.Trigger()
					}
				case <-exitCh:
					break addrLoop
				}
			}
		}
	}()

	return nil
}

func (c *CtrlHub) handleVxlanInterfaceNeighEvent() error {

	ipSearch := func(ip net.IP, link netlink.Link) error {
		var vtepMac net.HardwareAddr

		if mac, exist := c.nodeIPCache.SearchIP(ip); exist {
			// find node ip.
			vtepMac = mac
		} else {
			ipInstance, err := c.getIPInstanceByAddress(ip)
			if err != nil {
				return fmt.Errorf("failed to get ip instance by address %v: %v", ip.String(), err)
			}

			if ipInstance != nil {
				node := &corev1.Node{}
				nodeName := ipInstance.Labels[constants.LabelNode]

				if err := c.mgr.GetClient().Get(context.TODO(), types.NamespacedName{Name: nodeName}, node); err != nil {
					return fmt.Errorf("failed to get node %v: %v", nodeName, err)
				}

				vtepMac, err = net.ParseMAC(node.Annotations[constants.AnnotationNodeVtepMac])
				if err != nil {
					return fmt.Errorf("failed to parse vtep mac %v: %v",
						node.Annotations[constants.AnnotationNodeVtepMac], err)
				}
			} else if feature.MultiClusterEnabled() {
				// try to find remote vtep according to pod ip
				vtep, err := c.getRemoteVtepByEndpointAddress(ip)
				if err != nil {
					return fmt.Errorf("failed to get remote vtep by address %s: %v", ip.String(), err)
				}

				if vtep != nil {
					vtepMac, err = net.ParseMAC(vtep.Spec.VTEPInfo.MAC)
					if err != nil {
						return fmt.Errorf("failed to parse vtep mac %v: %v", vtep.Spec.VTEPInfo.MAC, err)
					}
				}
			}
		}

		if len(vtepMac) == 0 {
			// if ip not exist, try to clear it's neigh entries
			if err := neigh.ClearStaleNeighEntryByIP(link.Attrs().Index, ip); err != nil {
				return fmt.Errorf("failed to clear stale neigh for link %v and ip %v: %v",
					link.Attrs().Name, ip.String(), err)
			}

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
			return fmt.Errorf("failed to set neigh %v: %v", neighEntry.String(), err)
		}

		c.logger.V(4).Info("neigh proxy resolve success", "ip", ip.String(), "mac", vtepMac.String())

		return nil
	}

	ipSearchExecWrapper := func(ip net.IP, link netlink.Link) {
		if err := ipSearch(ip, link); err != nil {
			// print as info
			c.logger.Info("failed to proxy resolve overlay neigh", "message", err)
		}
	}

	hostNetNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get root netns: %v", err)
	}

	timer := time.NewTicker(c.config.VxlanExpiredNeighCachesClearInterval)
	errorMessageWrapper := initErrorMessageWrapper("failed to handle vxlan interface neigh event: ")

	go func() {
		for {
			// Clear stale and failed neigh entries for vxlan interface at the first time.
			// Once neigh subscribe failed (include the goroutine exit), it's most probably because of
			// "No buffer space available" problem, we need to clean expired neigh caches.
			if err := clearVxlanExpiredNeighCaches(); err != nil {
				c.logger.Error(err, "failed to clear vxlan expired neigh caches")
			}

			neighCh := make(chan netlink.NeighUpdate, NeighUpdateChanSize)
			exitCh := make(chan struct{})

			errorCallback := func(err error) {
				c.logger.Error(err, "subscribe netlink neigh event exit with error")
				close(exitCh)
			}

			if err := netlink.NeighSubscribeWithOptions(neighCh, nil, netlink.NeighSubscribeOptions{
				Namespace:     &hostNetNs,
				ErrorCallback: errorCallback,
			}); err != nil {
				c.logger.Error(err, "failed to subscribe neigh update event")
				time.Sleep(NetlinkSubscribeRetryInterval)
				continue
			}

		neighLoop:
			for {
				select {
				case update := <-neighCh:
					if isNeighResolving(update.State) {
						if update.Type == syscall.RTM_DELNEIGH {
							continue
						}

						link, err := netlink.LinkByIndex(update.LinkIndex)
						if err != nil {
							c.logger.Error(err, errorMessageWrapper("failed to get link by index %v", update.LinkIndex))
							continue
						}

						if strings.Contains(link.Attrs().Name, containernetwork.VxlanLinkInfix) {
							go ipSearchExecWrapper(update.IP, link)
						}
					}
				case <-timer.C:
					if err := clearVxlanExpiredNeighCaches(); err != nil {
						c.logger.Error(err, "failed to clear vxlan expired neigh caches")
					}
				case <-exitCh:
					break neighLoop
				}
			}
		}
	}()

	return nil
}

func (c *CtrlHub) iptablesSyncLoop() {
	iptablesSyncFunc := func() error {
		c.iptablesV4Manager.Reset()
		c.iptablesV6Manager.Reset()

		networkList := &networkingv1.NetworkList{}
		if err := c.mgr.GetClient().List(context.TODO(), networkList); err != nil {
			return fmt.Errorf("failed to list network: %v", err)
		}

		var overlayExist bool
		for _, network := range networkList.Items {
			if networkingv1.GetNetworkType(&network) == networkingv1.NetworkTypeOverlay {
				netID := network.Spec.NetID

				overlayIfName, err := containernetwork.GenerateVxlanNetIfName(c.config.NodeVxlanIfName, netID)
				if err != nil {
					return fmt.Errorf("failed to generate vxlan forward node if name: %v", err)
				}

				c.iptablesV4Manager.SetOverlayIfName(overlayIfName)
				c.iptablesV6Manager.SetOverlayIfName(overlayIfName)

				overlayExist = true
				break
			}
		}

		// add local subnets
		if overlayExist {
			// Record node ips.
			nodeList := &corev1.NodeList{}
			if err := c.mgr.GetClient().List(context.TODO(), nodeList); err != nil {
				return fmt.Errorf("failed to list node: %v", err)
			}

			for _, node := range nodeList.Items {
				// Underlay only environment should node print output

				if node.Annotations[constants.AnnotationNodeVtepMac] == "" ||
					node.Annotations[constants.AnnotationNodeVtepIP] == "" ||
					node.Annotations[constants.AnnotationNodeLocalVxlanIPList] == "" {
					c.logger.Info("node's vtep information has not been updated", "node", node.Name)
					continue
				}

				nodeLocalVxlanIPStringList := strings.Split(node.Annotations[constants.AnnotationNodeLocalVxlanIPList], ",")
				for _, ipString := range nodeLocalVxlanIPStringList {
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
			subnetList := &networkingv1.SubnetList{}
			if err := c.mgr.GetClient().List(context.TODO(), subnetList); err != nil {
				return fmt.Errorf("failed to list subnet: %v", err)
			}

			for _, subnet := range subnetList.Items {
				_, cidr, err := net.ParseCIDR(subnet.Spec.Range.CIDR)
				if err != nil {
					return fmt.Errorf("failed to parse subnet cidr %v: %v", subnet.Spec.Range.CIDR, err)
				}

				network := &networkingv1.Network{}
				if err := c.mgr.GetClient().Get(context.TODO(), types.NamespacedName{Name: subnet.Spec.Network}, network); err != nil {
					return fmt.Errorf("failed to get network for subnet %v", subnet.Name)
				}

				iptablesManager := c.getIPtablesManager(subnet.Spec.Range.Version)
				iptablesManager.RecordSubnet(cidr, networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay)
			}

			if feature.MultiClusterEnabled() {
				// If remote overlay network des not exist, the rcmanager will not fetch
				// RemoteSubnet and RemoteVtep. Thus, existence check is redundant here.

				remoteSubnetList := &multiclusterv1.RemoteSubnetList{}
				if err := c.mgr.GetClient().List(context.TODO(), remoteSubnetList); err != nil {
					return fmt.Errorf("failed to list remote network: %v", err)
				}

				// Record remote vtep ip.
				vtepList := &multiclusterv1.RemoteVtepList{}
				if err := c.mgr.GetClient().List(context.TODO(), vtepList); err != nil {
					return fmt.Errorf("failed to list remote vtep: %v", err)
				}

				for _, vtep := range vtepList.Items {
					if _, exist := vtep.Annotations[constants.AnnotationNodeLocalVxlanIPList]; !exist {
						ip := net.ParseIP(vtep.Spec.VTEPInfo.IP)
						if ip.To4() != nil {
							// v4 address
							c.iptablesV4Manager.RecordRemoteNodeIP(ip)
						} else {
							// v6 address
							c.iptablesV6Manager.RecordRemoteNodeIP(ip)
						}
						continue
					}

					nodeLocalVxlanIPStringList := strings.Split(vtep.Annotations[constants.AnnotationNodeLocalVxlanIPList], ",")
					for _, ipString := range nodeLocalVxlanIPStringList {
						ip := net.ParseIP(ipString)
						if ip.To4() != nil {
							// v4 address
							c.iptablesV4Manager.RecordRemoteNodeIP(ip)
						} else {
							// v6 address
							c.iptablesV6Manager.RecordRemoteNodeIP(ip)
						}
					}
				}

				// Record remote subnet cidr
				for _, remoteSubnet := range remoteSubnetList.Items {
					_, cidr, err := net.ParseCIDR(remoteSubnet.Spec.Range.CIDR)
					if err != nil {
						return fmt.Errorf("failed to parse remote subnet cidr %v: %v", remoteSubnet.Spec.Range.CIDR, err)
					}

					c.getIPtablesManager(remoteSubnet.Spec.Range.Version).
						RecordRemoteSubnet(cidr, multiclusterv1.GetRemoteSubnetType(&remoteSubnet) == networkingv1.NetworkTypeOverlay)
				}
			}
		}

		// Sync rules.
		if err := c.iptablesV4Manager.SyncRules(); err != nil {
			return fmt.Errorf("failed to sync v4 iptables rule: %v", err)
		}

		globalDisabled, err := containernetwork.CheckIPv6GlobalDisabled()
		if err != nil {
			return fmt.Errorf("failed to check ipv6 global disabled: %v", err)
		}

		if !globalDisabled {
			if err := c.iptablesV6Manager.SyncRules(); err != nil {
				return fmt.Errorf("failed to sync v6 iptables rule: %v", err)
			}
		}

		return nil
	}

	go func() {
		for {
			select {
			case <-c.iptablesSyncCh:
				if err := iptablesSyncFunc(); err != nil {
					c.logger.Error(err, "failed to sync iptables rule")
				}
			case <-c.iptablesSyncTicker.C:
				c.iptablesSyncTrigger()
			}
		}
	}()
}

func (c *CtrlHub) iptablesSyncTrigger() {
	select {
	case c.iptablesSyncCh <- struct{}{}:
	default:
	}
}

func (c *CtrlHub) runHealthyServer() {
	health := healthcheck.NewHandler()

	go func() {
		_ = http.ListenAndServe(c.config.HealthyServerAddress, health)
	}()

	c.logger.Info("start healthy server", "bind-address", c.config.HealthyServerAddress)
}

func isNeighResolving(state int) bool {
	// We need a neigh cache to be STALE if it's not used for a while.
	return (state & netlink.NUD_INCOMPLETE) != 0
}

func instanceIPIndexer(obj client.Object) []string {
	instance, ok := obj.(*networkingv1.IPInstance)
	if ok {
		podIP, _, err := net.ParseCIDR(instance.Spec.Address.IP)
		if err != nil {
			return []string{}
		}

		return []string{podIP.String()}
	}
	return []string{}
}

func endpointIPIndexer(obj client.Object) []string {
	vtep, ok := obj.(*multiclusterv1.RemoteVtep)
	if ok {
		endpointIPs := vtep.Spec.EndpointIPList
		if len(endpointIPs) > 0 {
			return endpointIPs
		}
	}
	return []string{}
}

func clearVxlanExpiredNeighCaches() error {
	linkList, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("failed to list link: %v", err)
	}

	for _, link := range linkList {
		if strings.Contains(link.Attrs().Name, containernetwork.VxlanLinkInfix) {
			if err := neigh.ClearStaleAddFailedNeighEntries(link.Attrs().Index, netlink.FAMILY_V4); err != nil {
				return fmt.Errorf("failed to clear v4 expired neigh entries for link %v: %v",
					link.Attrs().Name, err)
			}

			ipv6Disabled, err := containernetwork.CheckIPv6Disabled(link.Attrs().Name)
			if err != nil {
				return fmt.Errorf("failed to check ipv6 disables for link %v: %v", link.Attrs().Name, err)
			}

			if !ipv6Disabled {
				if err := neigh.ClearStaleAddFailedNeighEntries(link.Attrs().Index, netlink.FAMILY_V6); err != nil {
					return fmt.Errorf("failed to clear v6 expired neigh entries for link %v: %v",
						link.Attrs().Name, err)
				}
			}
		}
	}

	return nil
}
