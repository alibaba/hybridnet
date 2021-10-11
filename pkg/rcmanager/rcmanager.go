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

package rcmanager

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1 "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned/scheme"
	"github.com/alibaba/hybridnet/pkg/client/informers/externalversions"
	listers "github.com/alibaba/hybridnet/pkg/client/listers/networking/v1"
	"github.com/alibaba/hybridnet/pkg/utils"
)

const (
	ControllerName = "RemoteClusterManager"
	UserAgentName  = ControllerName
)

// Manager Those without the localCluster prefix are the resources of the remote cluster
type Manager struct {
	Meta
	LocalClusterKubeClient      kubeclientset.Interface
	LocalClusterHybridnetClient versioned.Interface
	RemoteSubnetLister          listers.RemoteSubnetLister
	RemoteVtepLister            listers.RemoteVtepLister
	LocalClusterSubnetLister    listers.SubnetLister

	KubeClient               *kubeclientset.Clientset
	HybridnetClient          *versioned.Clientset
	KubeInformerFactory      informers.SharedInformerFactory
	HybridnetInformerFactory externalversions.SharedInformerFactory
	NodeLister               corev1.NodeLister
	NodeSynced               cache.InformerSynced
	NodeQueue                workqueue.RateLimitingInterface
	NetworkLister            listers.NetworkLister
	NetworkSynced            cache.InformerSynced
	SubnetLister             listers.SubnetLister
	SubnetSynced             cache.InformerSynced
	SubnetQueue              workqueue.RateLimitingInterface
	IPLister                 listers.IPInstanceLister
	IPSynced                 cache.InformerSynced
	IPQueue                  workqueue.RateLimitingInterface
	RemoteClusterNodeLister  corev1.NodeLister
	RemoteClusterNodeSynced  cache.InformerSynced
	Recorder                 record.EventRecorder
}

type Meta struct {
	ClusterName string
	// RemoteClusterUID is used to set owner reference
	RemoteClusterUID types.UID
	// ClusterUUID represents the corresponding remote cluster's uuid, which is generated
	// from unique k8s resource
	ClusterUUID types.UID
	StopCh      chan struct{}
	// Only if meet the condition, can create remote cluster's cr
	// Conditions are:
	// 1. The remote cluster created the remote-cluster-cr of this cluster
	// 2. The remote cluster and local cluster both have overlay network
	// 3. The overlay network id is same with local cluster
	IsReady     bool
	IsReadyLock sync.RWMutex
}

func NewRemoteClusterManager(rc *networkingv1.RemoteCluster,
	localClusterKubeClient kubeclientset.Interface,
	localClusterHybridnetClient versioned.Interface,
	remoteSubnetLister listers.RemoteSubnetLister,
	localClusterSubnetLister listers.SubnetLister,
	remoteVtepLister listers.RemoteVtepLister) (*Manager, error) {
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("Panic hanppened. Can't new remote cluster manager. Maybe wrong kube config. "+
				"err=%v. remote cluster=%v\n%v", err, utils.ToJSONString(rc), debug.Stack())
		}
	}()
	klog.Infof("NewRemoteClusterManager %v", rc.Name)

	config, err := utils.BuildClusterConfig(rc)
	if err != nil {
		return nil, err
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: localClusterKubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: ControllerName})

	kubeClient := kubeclientset.NewForConfigOrDie(config)
	hybridnetClient := versioned.NewForConfigOrDie(restclient.AddUserAgent(config, UserAgentName))
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 0)
	hybridnetInformerFactory := externalversions.NewSharedInformerFactory(hybridnetClient, 0)

	nodeInformer := kubeInformerFactory.Core().V1().Nodes()
	networkInformer := hybridnetInformerFactory.Networking().V1().Networks()
	subnetInformer := hybridnetInformerFactory.Networking().V1().Subnets()
	ipInformer := hybridnetInformerFactory.Networking().V1().IPInstances()

	uuid, err := utils.GetUUID(kubeClient)
	if err != nil {
		return nil, err
	}

	stopCh := make(chan struct{})

	rcMgr := &Manager{
		Meta: Meta{
			ClusterName:      rc.Name,
			RemoteClusterUID: rc.UID,
			ClusterUUID:      uuid,
			StopCh:           stopCh,
			IsReady:          false,
		},
		LocalClusterKubeClient:      localClusterKubeClient,
		LocalClusterHybridnetClient: localClusterHybridnetClient,
		RemoteSubnetLister:          remoteSubnetLister,
		LocalClusterSubnetLister:    localClusterSubnetLister,
		RemoteVtepLister:            remoteVtepLister,
		KubeClient:                  kubeClient,
		HybridnetClient:             hybridnetClient,
		KubeInformerFactory:         kubeInformerFactory,
		HybridnetInformerFactory:    hybridnetInformerFactory,
		NodeLister:                  kubeInformerFactory.Core().V1().Nodes().Lister(),
		NodeSynced:                  kubeInformerFactory.Core().V1().Nodes().Informer().HasSynced,
		NodeQueue:                   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), fmt.Sprintf("%v-node", rc.ClusterName)),
		NetworkLister:               networkInformer.Lister(),
		NetworkSynced:               networkInformer.Informer().HasSynced,
		SubnetLister:                subnetInformer.Lister(),
		SubnetSynced:                subnetInformer.Informer().HasSynced,
		SubnetQueue:                 workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), fmt.Sprintf("%v-subnet", rc.ClusterName)),
		IPLister:                    ipInformer.Lister(),
		IPSynced:                    ipInformer.Informer().HasSynced,
		IPQueue:                     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), fmt.Sprintf("%v-ipinstance", rc.ClusterName)),
		RemoteClusterNodeLister:     nodeInformer.Lister(),
		RemoteClusterNodeSynced:     nodeInformer.Informer().HasSynced,
		Recorder:                    recorder,
	}

	nodeInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: rcMgr.filterNode,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    rcMgr.addOrDelNode,
			UpdateFunc: rcMgr.updateNode,
			DeleteFunc: rcMgr.addOrDelNode,
		},
	})

	subnetInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: rcMgr.filterSubnet,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    rcMgr.addOrDelSubnet,
			UpdateFunc: rcMgr.updateSubnet,
			DeleteFunc: rcMgr.addOrDelSubnet,
		},
	})

	ipInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: rcMgr.filterIPInstance,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    rcMgr.addOrDelIPInstance,
			UpdateFunc: rcMgr.updateIPInstance,
			DeleteFunc: rcMgr.addOrDelIPInstance,
		},
	})
	klog.Infof("Successfully New Remote Cluster Manager. Cluster=%v", rc.Name)
	return rcMgr, nil
}

func (m *Manager) Run() {
	klog.Infof("Start single remote cluster manager. clusterName=%v", m.ClusterName)

	managerCh := m.StopCh
	go func() {
		if ok := cache.WaitForCacheSync(managerCh, m.NodeSynced, m.SubnetSynced, m.IPSynced, m.NetworkSynced, m.RemoteClusterNodeSynced); !ok {
			klog.Errorf("failed to wait for remote cluster caches to sync. clusterName=%v", m.ClusterName)
			return
		}
		go wait.Until(m.RunNodeWorker, 1*time.Second, managerCh)
		go wait.Until(m.RunSubnetWorker, 1*time.Second, managerCh)
		go wait.Until(m.RunIPInstanceWorker, 1*time.Second, managerCh)
	}()
	go m.KubeInformerFactory.Start(managerCh)
	go m.HybridnetInformerFactory.Start(managerCh)
}

func (m *Manager) GetIsReady() bool {
	m.IsReadyLock.RLock()
	defer m.IsReadyLock.RUnlock()

	return m.IsReady
}

func (m *Manager) SetIsReady(val bool) {
	m.IsReadyLock.Lock()
	defer m.IsReadyLock.Unlock()

	m.IsReady = val
}

func (m *Manager) Close() {
	close(m.StopCh)
}
