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

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/rcmanager"
	"github.com/oecp/rama/pkg/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog"
)

func (c *Controller) startRemoteClusterMgr(clusterName string) error {
	klog.Infof("processNextRemoteClusterMgr name=%v", clusterName)
	rcManager, exists := c.rcMgrCache.Get(clusterName)
	if !exists {
		klog.Errorf("Can't find rcManager. clusterName=%v", clusterName)
		return errors.Errorf("Can't find rcManager. clusterName=%v", clusterName)
	}
	rcManager.Run()
	return nil
}

// use remove+add instead of update
func (c *Controller) addOrUpdateRCMgr(rc *networkingv1.RemoteCluster) error {
	// lock in function range to avoid renewing cluster manager when newing one
	c.rcMgrCache.RWMutex.Lock()
	defer c.rcMgrCache.RWMutex.Unlock()
	klog.Infof("[addOrUpdateRCMgr] cluster=%v", rc.Name)

	clusterName := rc.Name
	if mgr, exists := c.rcMgrCache.rcMgrMap[clusterName]; exists {
		klog.Infof("Delete cluster %v from cache", clusterName)
		mgr.Close()
		delete(c.rcMgrCache.rcMgrMap, clusterName)
	}

	rcMgr, err := rcmanager.NewRemoteClusterManager(rc, c.kubeClient, c.ramaClient, c.remoteSubnetLister,
		c.localClusterSubnetLister, c.remoteVtepLister)

	conditions := make([]networkingv1.ClusterCondition, 0)
	if err != nil || rcMgr == nil || rcMgr.RamaClient == nil || rcMgr.KubeClient == nil {
		connErr := errors.Errorf("Can't connect to remote cluster %v", clusterName)
		c.recorder.Eventf(rc, corev1.EventTypeWarning, "ErrClusterConnectionConfig", connErr.Error())
		conditions = append(conditions, utils.NewClusterOffline(connErr))
	} else {
		conditions = CheckCondition(c, rcMgr.RamaClient, rc.ClusterName, DefaultChecker)
		rc.Status.UUID = rcMgr.ClusterUUID
	}
	rc.Status.Conditions = conditions

	_, err = c.ramaClient.NetworkingV1().RemoteClusters().UpdateStatus(context.TODO(), rc, metav1.UpdateOptions{})
	if err != nil {
		runtime.HandleError(err)
		return err
	}
	rcMgr.SetIsReady(IsReady(conditions))

	c.rcMgrCache.rcMgrMap[clusterName] = rcMgr
	c.rcMgrQueue.Add(clusterName)
	return nil
}

func (c *Controller) processRCManagerQueue() {
	for c.processNextRemoteClusterMgr() {
	}
}

func (c *Controller) processNextRemoteClusterMgr() bool {
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("processNextRemoteClusterMgr panic. err=%v", err)
		}
	}()

	obj, shutdown := c.rcMgrQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.rcMgrQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			c.rcMgrQueue.Forget(obj)
			return nil
		}
		if err := c.startRemoteClusterMgr(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.rcMgrQueue.AddRateLimited(key)
			return fmt.Errorf("[remote cluster mgr] fail to sync '%v': %v, requeuing", key, err)
		}
		c.rcMgrQueue.Forget(obj)
		klog.Infof("[remote-cluster-manager] succeed to sync '%v'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}
