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

package ipam

import (
	"fmt"
	"time"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/client/clientset/versioned"
	informers "github.com/oecp/rama/pkg/client/informers/externalversions/networking/v1"
	listers "github.com/oecp/rama/pkg/client/listers/networking/v1"
	"github.com/oecp/rama/pkg/feature"
	"github.com/oecp/rama/pkg/ipam"
	"github.com/oecp/rama/pkg/ipam/allocator"
	"github.com/oecp/rama/pkg/ipam/store"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	informerscorev1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const ControllerName = "ipam"

type Controller struct {
	kubeClientSet       kubernetes.Interface
	networkingClientSet versioned.Interface

	podLister corev1.PodLister
	podSynced cache.InformerSynced

	nodeLister corev1.NodeLister
	nodeSynced cache.InformerSynced

	networkLister listers.NetworkLister
	networkSynced cache.InformerSynced

	subnetLister listers.SubnetLister
	subnetSynced cache.InformerSynced

	ipLister listers.IPInstanceLister
	ipSynced cache.InformerSynced

	// should be initialized after running
	ipamManager          ipam.Interface
	dualStackIPAMManager ipam.DualStackInterface

	ipamStore          ipam.Store
	dualStackIPAMStroe ipam.DualStackStore

	// for pod IP allocation
	podQueue workqueue.RateLimitingInterface

	// for node condition
	nodeQueue workqueue.RateLimitingInterface

	// for IP pre-assign and recycle
	ipQueue workqueue.RateLimitingInterface

	// for network refresh
	networkQueue workqueue.RateLimitingInterface

	// for network status update
	networkStatusQueue workqueue.RateLimitingInterface

	ipamCache       *Cache
	ipamUsageSyncer *UsageSyncer

	recorder record.EventRecorder
}

func NewController(
	kubeClientSet kubernetes.Interface,
	networkingClientSet versioned.Interface,
	podInformer informerscorev1.PodInformer,
	nodeInformer informerscorev1.NodeInformer,
	networkInformer informers.NetworkInformer,
	subnetInformer informers.SubnetInformer,
	ipInformer informers.IPInstanceInformer) *Controller {
	runtime.Must(networkingv1.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientSet.CoreV1().Events("")})

	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: ControllerName})

	c := &Controller{
		kubeClientSet:        kubeClientSet,
		networkingClientSet:  networkingClientSet,
		podLister:            podInformer.Lister(),
		podSynced:            podInformer.Informer().HasSynced,
		nodeLister:           nodeInformer.Lister(),
		nodeSynced:           nodeInformer.Informer().HasSynced,
		networkLister:        networkInformer.Lister(),
		networkSynced:        networkInformer.Informer().HasSynced,
		subnetLister:         subnetInformer.Lister(),
		subnetSynced:         subnetInformer.Informer().HasSynced,
		ipLister:             ipInformer.Lister(),
		ipSynced:             ipInformer.Informer().HasSynced,
		ipamManager:          nil,
		dualStackIPAMManager: nil,
		ipamStore:            store.NewWorker(kubeClientSet, networkingClientSet),
		dualStackIPAMStroe:   store.NewDualStackWorker(kubeClientSet, networkingClientSet),
		podQueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pod"),
		nodeQueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "node"),
		ipQueue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ip"),
		networkQueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "network"),
		networkStatusQueue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "network-status"),
		ipamCache:            NewCache(),
		recorder:             recorder,
	}

	klog.Info("Setting up event handlers")

	podInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: c.filterPod,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addPod,
			UpdateFunc: c.updatePod,
		},
	})

	networkInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: c.filterNetwork,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addNetwork,
			UpdateFunc: c.updateNetwork,
			DeleteFunc: c.delNetwork,
		},
	})

	subnetInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: c.filterSubnet,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addSubnet,
			UpdateFunc: c.updateSubnet,
			DeleteFunc: c.delSubnet,
		},
	})

	ipInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: c.filterIP,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addIP,
			UpdateFunc: c.updateIP,
		},
	})

	nodeInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: c.filterNode,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addNode,
			UpdateFunc: c.updateNode,
			DeleteFunc: c.delNode,
		},
	})
	return c
}

func (c *Controller) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.networkStatusQueue.ShutDown()
	defer c.networkQueue.ShutDown()
	defer c.nodeQueue.ShutDown()
	defer c.podQueue.ShutDown()
	defer c.ipQueue.ShutDown()

	klog.Infof("Starting %s controller", ControllerName)

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.networkSynced, c.subnetSynced, c.ipSynced, c.podSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	// init IPAM manager
	klog.Info("Initializing IPAM manager")

	if err := c.initializeIPAMManager(); err != nil {
		return fmt.Errorf("failed to initialize IPAM manager: %v", err)
	}

	// start workers
	klog.Info("Starting workers")

	go wait.Until(c.runNetworkStatusWorker, time.Second, stopCh)
	go wait.Until(c.runNetworkWorker, time.Second, stopCh)
	go wait.Until(c.runNodeWorker, time.Second, stopCh)
	go wait.Until(c.runPodWorker, time.Second, stopCh)
	go wait.Until(c.runIPWorker, time.Second, stopCh)

	// run Status Syncer
	c.runIPAMUsageSyncer(stopCh, time.Minute)

	klog.Info("Started workers")

	<-stopCh

	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) initializeIPAMManager() error {
	networks, err := c.networkLister.List(labels.Everything())
	if err != nil {
		return err
	}

	var networkNames []string
	for _, network := range networks {
		networkNames = append(networkNames, network.Name)
	}

	if feature.DualStackEnabled() {
		alc, err := allocator.NewDualStackAllocator(networkNames, c.networkGetter, c.subnetGetter, c.ipSetGetter)
		if err != nil {
			return err
		}
		c.dualStackIPAMManager = alc
	} else {
		alc, err := allocator.NewAllocator(networkNames, c.networkGetter, c.subnetGetter, c.ipSetGetter)
		if err != nil {
			return err
		}
		c.ipamManager = alc
	}

	return nil
}

func (c *Controller) runIPAMUsageSyncer(stopCh <-chan struct{}, period time.Duration) {
	if feature.DualStackEnabled() {
		c.ipamUsageSyncer = NewDualStackUsageSyncer(c.dualStackIPAMManager, c.dualStackIPAMStroe, c.ipamCache)
	} else {
		c.ipamUsageSyncer = NewUsageSyncer(c.ipamManager, c.ipamStore, c.ipamCache)
	}

	c.ipamUsageSyncer.Run(stopCh, period)
}

func (c *Controller) runNetworkWorker() {
	for c.processNextNetwork() {
	}
}

func (c *Controller) processNextNetwork() bool {
	obj, shutdown := c.networkQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.networkQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.networkQueue.Forget(obj)
			return nil
		}
		if err := c.reconcileNetwork(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.networkQueue.AddRateLimited(key)
			return fmt.Errorf("[network] fail to sync '%s': %v, requeuing", key, err)
		}
		c.networkQueue.Forget(obj)
		klog.Infof("[network] succeed to sync '%s'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}

func (c *Controller) runNetworkStatusWorker() {
	for c.processNextNetworkStatus() {
	}
}

func (c *Controller) processNextNetworkStatus() bool {
	obj, shutdown := c.networkStatusQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.networkStatusQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.networkStatusQueue.Forget(obj)
			return nil
		}
		if err := c.reconcileNetworkStatus(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.networkStatusQueue.AddRateLimited(key)
			return fmt.Errorf("[network-status] fail to sync '%s': %v, requeuing", key, err)
		}
		c.networkStatusQueue.Forget(obj)
		klog.Infof("[network-status] succeed to sync '%s'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}

func (c *Controller) runPodWorker() {
	for c.processNextPod() {
	}
}

func (c *Controller) processNextPod() bool {
	obj, shutdown := c.podQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.podQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.podQueue.Forget(obj)
			return nil
		}
		if err := c.reconcilePod(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.podQueue.AddRateLimited(key)
			return fmt.Errorf("[pod] fail to sync '%s': %v, requeuing", key, err)
		}
		c.podQueue.Forget(obj)
		klog.Infof("[pod] succeed to sync '%s'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}

func (c *Controller) runNodeWorker() {
	for c.processNextNode() {
	}
}

func (c *Controller) processNextNode() bool {
	obj, shutdown := c.nodeQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.nodeQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.nodeQueue.Forget(obj)
			return nil
		}
		if err := c.reconcileNode(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.nodeQueue.AddRateLimited(key)
			return fmt.Errorf("[node] fail to sync '%s': %v, requeuing", key, err)
		}
		c.nodeQueue.Forget(obj)
		klog.Infof("[node] succeed to sync '%s'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}

func (c *Controller) runIPWorker() {
	for c.processNextIP() {
	}
}

func (c *Controller) processNextIP() bool {
	obj, shutdown := c.ipQueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.ipQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.ipQueue.Forget(obj)
			return nil
		}
		if err := c.reconcileIP(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.ipQueue.AddRateLimited(key)
			return fmt.Errorf("[ip] fail to sync '%s': %v, requeuing", key, err)
		}
		c.ipQueue.Forget(obj)
		klog.Infof("[ip] succeed to sync '%s'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}
