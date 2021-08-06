package remotecluster

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/client/clientset/versioned"
	"github.com/oecp/rama/pkg/client/informers/externalversions"
	informers "github.com/oecp/rama/pkg/client/informers/externalversions/networking/v1"
	listers "github.com/oecp/rama/pkg/client/listers/networking/v1"
	"github.com/oecp/rama/pkg/metrics"
	"github.com/oecp/rama/pkg/rcmanager"
	"github.com/oecp/rama/pkg/utils"
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
)

const (
	ControllerName = "remotecluster"

	// HealthCheckPeriod Every HealthCheckPeriod will resync remote cluster cache and check rc
	// health. Default: 20 second. Set to zero will also use the default value
	HealthCheckPeriod = 20 * time.Second
)

type Controller struct {
	// localCluster's UUID
	UUID                      types.UID
	OverlayNetID              *uint32
	overlayNetIDMU            sync.RWMutex
	rcMgrCache                Cache
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

	recorder   record.EventRecorder
	rcMgrQueue workqueue.RateLimitingInterface
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
		rcMgrCache: Cache{
			rcMgrMap: make(map[string]*rcmanager.Manager),
		},
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
		rcMgrQueue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "remoteclustermanager"),
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

	return c
}

func (c *Controller) Run(stopCh <-chan struct{}) error {
	defer runtimeutil.HandleCrash()
	defer c.rcMgrQueue.ShutDown()
	defer c.remoteClusterQueue.ShutDown()

	klog.Infof("Starting %s controller", ControllerName)

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.remoteClusterSynced, c.remoteSubnetSynced, c.remoteVtepSynced, c.localClusterSubnetSynced); !ok {
		return fmt.Errorf("%s failed to wait for caches to sync", ControllerName)
	}

	// start workers
	klog.Info("Starting workers")
	go wait.Until(c.runRemoteClusterWorker, time.Second, stopCh)
	go wait.Until(c.processRCManagerQueue, time.Second, stopCh)
	go wait.Until(c.runOverlayNetIDWorker, time.Minute, stopCh)
	go wait.Until(c.updateRemoteClusterStatus, HealthCheckPeriod, stopCh)
	<-stopCh

	c.closeRemoteClusterManager()

	klog.Info("Shutting down workers")
	return nil
}

func (c *Controller) closeRemoteClusterManager() {
	// no need to lock
	for _, wrapper := range c.rcMgrCache.rcMgrMap {
		close(wrapper.StopCh)
	}
}

func (c *Controller) runOverlayNetIDWorker() {
	c.overlayNetIDMU.Lock()
	defer c.overlayNetIDMU.Unlock()

	networks, err := c.localClusterNetworkLister.List(labels.NewSelector())
	if err != nil {
		klog.Warningf("Can't list local cluster network. err=%v", err)
	}
	for _, network := range networks {
		if network.Spec.Type == networkingv1.NetworkTypeOverlay {
			n := network.DeepCopy()
			c.OverlayNetID = n.Spec.NetID
			break
		}
	}
}

// health checking and resync cache. remote cluster is managed by admin, it can be
// treated as desired states
func (c *Controller) updateRemoteClusterStatus() {
	remoteClusters, err := c.remoteClusterLister.List(labels.NewSelector())
	if err != nil {
		klog.Errorf("Can't list remote cluster. err=%v", err)
		return
	}

	var (
		wg  sync.WaitGroup
		cnt = 0
	)
	for _, rc := range remoteClusters {
		r := rc.DeepCopy()
		manager, exists := c.rcMgrCache.Get(r.Name)
		if !exists {
			continue
		}
		cnt = cnt + 1
		wg.Add(1)
		go c.updateSingleRCStatus(manager, r, &wg)
	}
	wg.Wait()
	klog.Infof("Update Remote Cluster Status Finished. len=%v", cnt)
}

func (c *Controller) updateSingleRCStatus(manager *rcmanager.Manager, rc *networkingv1.RemoteCluster, wg *sync.WaitGroup) {
	rc = rc.DeepCopy()
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("updateSingleRCStatus panic. err=%v\n%v", err, string(debug.Stack()))
		}
	}()
	defer metrics.RemoteClusterStatusUpdateDurationFromStart(time.Now())
	defer wg.Done()

	manager.IsReadyLock.Lock()
	defer manager.IsReadyLock.Unlock()

	conditions := CheckCondition(c, manager.RamaClient, manager.ClusterName, InitializeChecker)
	newIsReady := IsReady(conditions)
	if manager.IsReady == false && newIsReady {
		manager.IsReady = true
		ResumeReconcile(manager)
	}

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
