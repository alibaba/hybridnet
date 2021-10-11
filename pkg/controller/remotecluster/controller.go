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

package remotecluster

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	"github.com/alibaba/hybridnet/pkg/client/informers/externalversions"
	informers "github.com/alibaba/hybridnet/pkg/client/informers/externalversions/networking/v1"
	listers "github.com/alibaba/hybridnet/pkg/client/listers/networking/v1"
	"github.com/alibaba/hybridnet/pkg/metrics"
	"github.com/alibaba/hybridnet/pkg/rcmanager"
	"github.com/alibaba/hybridnet/pkg/utils"
)

const (
	ControllerName = "remotecluster"

	// HealthCheckPeriod Every HealthCheckPeriod will resync remote cluster cache and check rc
	// health. Default: 20 second. Set to zero will also use the default value
	HealthCheckPeriod = 20 * time.Second
)

type Controller struct {
	// localCluster's UUID
	UUID           types.UID
	OverlayNetID   *uint32
	overlayNetIDMU sync.RWMutex

	rcManagerCache sync.Map

	kubeClient                kubeclientset.Interface
	ramaClient                versioned.Interface
	RamaInformerFactory       externalversions.SharedInformerFactory
	remoteClusterLister       listers.RemoteClusterLister
	remoteClusterSynced       cache.InformerSynced
	remoteClusterQueue        workqueue.RateLimitingInterface
	remoteSubnetLister        listers.RemoteSubnetLister
	remoteSubnetSynced        cache.InformerSynced
	remoteVtepLister          listers.RemoteVtepLister
	remoteVtepSynced          cache.InformerSynced
	localClusterSubnetLister  listers.SubnetLister
	localClusterSubnetSynced  cache.InformerSynced
	localClusterNetworkLister listers.NetworkLister
	localClusterNetworkSynced cache.InformerSynced

	recorder record.EventRecorder
}

func NewController(
	kubeClient kubeclientset.Interface,
	ramaClient versioned.Interface,
	remoteClusterInformer informers.RemoteClusterInformer,
	remoteSubnetInformer informers.RemoteSubnetInformer,
	localClusterSubnetInformer informers.SubnetInformer,
	remoteVtepInformer informers.RemoteVtepInformer,
	localClusterNetworkInformer informers.NetworkInformer) *Controller {
	runtimeutil.Must(networkingv1.AddToScheme(scheme.Scheme))

	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: ControllerName})

	uuid, err := utils.GetUUID(kubeClient)
	if err != nil {
		panic(err)
	}

	c := &Controller{
		rcManagerCache:            sync.Map{},
		UUID:                      uuid,
		kubeClient:                kubeClient,
		ramaClient:                ramaClient,
		remoteClusterLister:       remoteClusterInformer.Lister(),
		remoteClusterSynced:       remoteClusterInformer.Informer().HasSynced,
		remoteSubnetLister:        remoteSubnetInformer.Lister(),
		remoteSubnetSynced:        remoteSubnetInformer.Informer().HasSynced,
		localClusterSubnetLister:  localClusterSubnetInformer.Lister(),
		localClusterSubnetSynced:  localClusterSubnetInformer.Informer().HasSynced,
		remoteVtepLister:          remoteVtepInformer.Lister(),
		remoteVtepSynced:          remoteSubnetInformer.Informer().HasSynced,
		localClusterNetworkLister: localClusterNetworkInformer.Lister(),
		localClusterNetworkSynced: localClusterNetworkInformer.Informer().HasSynced,
		remoteClusterQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), ControllerName),
		recorder:                  recorder,
	}

	remoteClusterInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: c.filterRemoteCluster,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addOrDelRemoteCluster,
			UpdateFunc: c.updateRemoteCluster,
			DeleteFunc: c.addOrDelRemoteCluster,
		},
	})

	localClusterNetworkInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			network, ok := obj.(*networkingv1.Network)
			if !ok {
				return false
			}
			return network.Spec.Type == networkingv1.NetworkTypeOverlay
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(_ interface{}) {
				c.syncLocalOverlayNetIDOnce()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldNetwork, _ := oldObj.(*networkingv1.Network)
				newNetwork, _ := newObj.(*networkingv1.Network)
				needSync := oldNetwork.Spec.Type != newNetwork.Spec.Type || oldNetwork.Spec.NetID == nil ||
					newNetwork.Spec.NetID == nil || *oldNetwork.Spec.NetID != *newNetwork.Spec.NetID

				if needSync {
					c.syncLocalOverlayNetIDOnce()
				}
			},
			DeleteFunc: func(_ interface{}) {
				c.syncLocalOverlayNetIDOnce()
			},
		},
	})

	return c
}

func (c *Controller) Run(stopCh <-chan struct{}) error {
	defer runtimeutil.HandleCrash()
	defer c.remoteClusterQueue.ShutDown()

	klog.Infof("Starting %s controller", ControllerName)

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.remoteClusterSynced, c.remoteSubnetSynced, c.remoteVtepSynced, c.localClusterSubnetSynced, c.localClusterNetworkSynced); !ok {
		return fmt.Errorf("%s failed to wait for caches to sync", ControllerName)
	}

	// start workers
	klog.Info("Starting workers")
	go wait.Until(c.runRemoteClusterWorker, time.Second, stopCh)
	go wait.Until(c.updateRemoteClusterStatus, HealthCheckPeriod, stopCh)
	<-stopCh

	c.closeAllRemoteClusterManager()

	klog.Info("Shutting down workers")
	return nil
}

func (c *Controller) closeAllRemoteClusterManager() {
	c.rcManagerCache.Range(func(_, value interface{}) bool {
		if manager, ok := value.(*rcmanager.Manager); ok {
			manager.Close()
		}
		return true
	})
}

func (c *Controller) syncLocalOverlayNetIDOnce() {
	c.overlayNetIDMU.Lock()
	defer c.overlayNetIDMU.Unlock()

	networks, err := c.localClusterNetworkLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("failed to list networks: %v", err)
		return
	}

	var overlayNetworkExist = false
	for _, network := range networks {
		if network.Spec.Type == networkingv1.NetworkTypeOverlay {
			overlayNetworkExist = true
			switch {
			case c.OverlayNetID == nil || network.Spec.NetID == nil:
				fallthrough
			case *c.OverlayNetID != *network.Spec.NetID:
				c.OverlayNetID = copyUint32Ptr(network.Spec.NetID)
			}
			break
		}
	}

	// clean overlay netID cache if non-existence
	if !overlayNetworkExist {
		c.OverlayNetID = nil
	}
}

// health checking and resync cache. remote cluster is managed by admin, it can be
// treated as desired states
func (c *Controller) updateRemoteClusterStatus() {
	remoteClusters, err := c.remoteClusterLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("Can't list remote cluster. err=%v", err)
		return
	}

	var wg sync.WaitGroup
	for _, rc := range remoteClusters {
		r := rc.DeepCopy()

		managerObject, ok := c.rcManagerCache.Load(r.Name)
		if !ok {
			continue
		}
		manager, ok := managerObject.(*rcmanager.Manager)
		if !ok {
			continue
		}

		wg.Add(1)
		go func() {
			c.updateSingleRCStatus(manager, r)
			wg.Done()
		}()
	}
	wg.Wait()
}

func (c *Controller) updateSingleRCStatus(manager *rcmanager.Manager, rc *networkingv1.RemoteCluster) {
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("updateSingleRCStatus panic. err=%v\n%v", err, string(debug.Stack()))
		}
	}()
	defer metrics.RemoteClusterStatusUpdateDurationFromStart(time.Now())

	manager.IsReadyLock.Lock()
	defer manager.IsReadyLock.Unlock()

	conditions := CheckCondition(c, manager.RamaClient, manager.ClusterName, DefaultChecker)
	newIsReady := IsReady(conditions)

	if !manager.IsReady && newIsReady {
		manager.IsReady = true
		ResumeReconcile(manager)
	}

	rc = rc.DeepCopy()
	updateLastTransitionTime := func() {
		conditionChanged := false
		if len(conditions) != len(rc.Status.Conditions) {
			conditionChanged = true
		} else {
			for i := range conditions {
				if conditions[i].Status == rc.Status.Conditions[i].Status &&
					conditions[i].Type == rc.Status.Conditions[i].Type {
					continue
				} else {
					conditionChanged = true
					break
				}
			}
		}
		if !conditionChanged {
			for i := range rc.Status.Conditions {
				conditions[i].LastTransitionTime = rc.Status.Conditions[i].LastTransitionTime
			}
		}
		rc.Status.Conditions = conditions
	}
	updateLastTransitionTime()

	_, err := c.ramaClient.NetworkingV1().RemoteClusters().UpdateStatus(context.TODO(), rc, metav1.UpdateOptions{})
	if err != nil {
		klog.Warningf("[updateSingleRCStatus] can't update remote cluster. err=%v", err)
	}
}

func ResumeReconcile(manager *rcmanager.Manager) {
	manager.EnqueueSubnet(rcmanager.ReconcileSubnet)
	manager.EnqueueNode(rcmanager.ReconcileNode)
}

func copyUint32Ptr(i *uint32) *uint32 {
	if i == nil {
		return nil
	}
	o := *i
	return &o
}
