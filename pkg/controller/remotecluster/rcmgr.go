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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/rcmanager"
	"github.com/alibaba/hybridnet/pkg/utils"
)

// use remove+add instead of update
func (c *Controller) addOrUpdateRCMgr(rc *networkingv1.RemoteCluster) error {
	klog.Infof("[addOrUpdateRCMgr] cluster=%v", rc.Name)

	clusterName := rc.Name
	if oldManagerObject, loaded := c.rcManagerCache.LoadAndDelete(clusterName); loaded {
		oldManager, ok := oldManagerObject.(*rcmanager.Manager)
		if ok {
			oldManager.Close()
		}
	}

	rcMgr, err := rcmanager.NewRemoteClusterManager(rc, c.kubeClient, c.hybridnetClient, c.remoteSubnetLister,
		c.localClusterSubnetLister, c.remoteVtepLister)

	conditions := make([]networkingv1.ClusterCondition, 0)
	if err != nil || rcMgr == nil || rcMgr.HybridnetClient == nil || rcMgr.KubeClient == nil {
		connErr := errors.Errorf("Can't connect to remote cluster %v", clusterName)
		c.recorder.Eventf(rc, corev1.EventTypeWarning, "ErrClusterConnectionConfig", connErr.Error())
		conditions = append(conditions, utils.NewClusterOffline(connErr))
	} else {
		conditions = CheckCondition(c, rcMgr.HybridnetClient, rc.ClusterName, DefaultChecker)
		rc.Status.UUID = rcMgr.ClusterUUID
	}
	rc.Status.Conditions = conditions

	_, err = c.hybridnetClient.NetworkingV1().RemoteClusters().UpdateStatus(context.TODO(), rc, metav1.UpdateOptions{})
	if err != nil {
		runtime.HandleError(err)
		return err
	}
	rcMgr.SetIsReady(IsReady(conditions))

	c.rcManagerCache.Store(clusterName, rcMgr)
	rcMgr.Run()
	return nil
}
