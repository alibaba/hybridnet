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
	"fmt"
	"reflect"

	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/rcmanager"
)

// remote cluster is managed by admin, no need to full synchronize
// FIXME: use finalizer to delete remote cluster gracefully
func (c *Controller) reconcileRemoteCluster(clusterName string) error {
	klog.Infof("Reconciling Remote Cluster %v", clusterName)
	remoteCluster, err := c.remoteClusterLister.Get(clusterName)
	if err != nil {
		if k8serror.IsNotFound(err) {
			if terminatingManagerObject, loaded := c.rcManagerCache.LoadAndDelete(clusterName); loaded {
				terminatingManager, ok := terminatingManagerObject.(*rcmanager.Manager)
				if ok {
					terminatingManager.Close()
				}
			}
			return nil
		}
		return err
	}
	return c.addOrUpdateRCMgr(remoteCluster)
}

func (c *Controller) filterRemoteCluster(obj interface{}) bool {
	_, ok := obj.(*networkingv1.RemoteCluster)
	return ok
}

func (c *Controller) addOrDelRemoteCluster(obj interface{}) {
	rc, _ := obj.(*networkingv1.RemoteCluster)
	c.enqueueRemoteCluster(rc.Name)
}

func (c *Controller) updateRemoteCluster(oldObj, newObj interface{}) {
	oldRC, _ := oldObj.(*networkingv1.RemoteCluster)
	newRC, _ := newObj.(*networkingv1.RemoteCluster)

	if oldRC.ResourceVersion == newRC.ResourceVersion ||
		oldRC.Generation == newRC.Generation {
		return
	}
	if !remoteClusterSpecChanged(&oldRC.Spec, &newRC.Spec) {
		return
	}
	c.enqueueRemoteCluster(newRC.ClusterName)
}

func (c *Controller) enqueueRemoteCluster(clusterName string) {
	c.remoteClusterQueue.Add(clusterName)
}

func (c *Controller) runRemoteClusterWorker() {
	for c.processNextRemoteCluster() {
	}
}

func (c *Controller) processNextRemoteCluster() bool {
	obj, shutdown := c.remoteClusterQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.remoteClusterQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			c.remoteClusterQueue.Forget(obj)
			return nil
		}
		if err := c.reconcileRemoteCluster(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.remoteClusterQueue.AddRateLimited(key)
			return fmt.Errorf("[remote cluster] fail to sync '%v': %v, requeuing", key, err)
		}
		c.remoteClusterQueue.Forget(obj)
		klog.Infof("[remote cluster] succeed to sync '%v'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}

func remoteClusterSpecChanged(old, new *networkingv1.RemoteClusterSpec) bool {
	return !reflect.DeepEqual(old.ConnConfig, new.ConnConfig)
}
