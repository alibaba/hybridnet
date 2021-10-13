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
	"runtime/debug"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controller/remotecluster/status"
	"github.com/alibaba/hybridnet/pkg/metrics"
	"github.com/alibaba/hybridnet/pkg/rcmanager"
)

func (c *Controller) syncRemoteClusterManager(remoteCluster *networkingv1.RemoteCluster) error {
	klog.Infof("[syncRemoteClusterManager] cluster=%v", remoteCluster.Name)

	clusterName := remoteCluster.Name
	if oldManagerObject, loaded := c.rcManagerCache.LoadAndDelete(clusterName); loaded {
		oldManager, ok := oldManagerObject.(*rcmanager.Manager)
		if ok {
			oldManager.Close()
		}
	}

	manager, err := rcmanager.NewRemoteClusterManager(remoteCluster, c.kubeClient, c.hybridnetClient, c.remoteSubnetLister,
		c.localClusterSubnetLister, c.remoteVtepLister)
	if err != nil {
		klog.Errorf("fail to create remote cluster manager: %v", err)
		c.recorder.Event(remoteCluster, corev1.EventTypeWarning, "ErrNewRemoteClusterManager", err.Error())
		return err
	}

	c.rcManagerCache.Store(clusterName, manager)
	manager.Run()
	return nil
}

func updateSingleRemoteClusterStatus(c *Controller, manager *rcmanager.Manager, rc *networkingv1.RemoteCluster) {
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("updateSingleRemoteClusterStatus panic. err=%v\n%v", err, string(debug.Stack()))
		}
	}()
	defer metrics.RemoteClusterStatusUpdateDurationFromStart(time.Now())

	manager.IsReadyLock.Lock()
	defer manager.IsReadyLock.Unlock()

	newRemoteCluster := rc.DeepCopy()
	clusterStatus := status.Check(c, manager, newRemoteCluster.Status.Conditions)
	newRemoteCluster.Status.Status = clusterStatus

	if !manager.IsReady && clusterStatus == networkingv1.ClusterReady {
		manager.IsReady = true
		resumeReconcile(manager)
	}

	_, err := c.hybridnetClient.NetworkingV1().RemoteClusters().UpdateStatus(context.TODO(), newRemoteCluster, metav1.UpdateOptions{})
	if err != nil {
		klog.Warningf("[updateSingleRemoteClusterStatus] can't update remote cluster: %v", err)
		c.recorder.Event(rc, corev1.EventTypeWarning, "UpdateStatusFail", err.Error())
	}
}

func resumeReconcile(manager *rcmanager.Manager) {
	manager.EnqueueSubnet(rcmanager.ReconcileSubnet)
	manager.EnqueueNode(rcmanager.ReconcileNode)
}
