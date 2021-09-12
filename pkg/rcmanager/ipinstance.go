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
	"context"
	"fmt"

	"github.com/gogf/gf/container/gset"
	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/utils"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

// reconcile one single node, update/add/remove everything about it's
// corresponding remote vtep.
func (m *Manager) reconcileIPInstance(nodeName string) error {
	klog.Infof("[remote cluster] Starting reconcile ipinstance from cluster %v, node name=%v", m.ClusterName, nodeName)
	if len(nodeName) == 0 {
		return nil
	}
	vtepName := utils.GenRemoteVtepName(m.ClusterName, nodeName)
	node, err := m.NodeLister.Get(nodeName)
	if err != nil {
		if k8serror.IsNotFound(err) {
			err = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().Delete(context.TODO(), vtepName, metav1.DeleteOptions{})
			if k8serror.IsNotFound(err) {
				return nil
			}
		}
		return err
	}
	vtepIP := node.Annotations[constants.AnnotationNodeVtepIP]
	vtepMac := node.Annotations[constants.AnnotationNodeVtepMac]
	instances, err := m.IPLister.List(labels.SelectorFromSet(labels.Set{constants.LabelNode: nodeName}))
	if err != nil {
		return err
	}
	newVtep := false

	remoteVtep, err := m.RemoteVtepLister.Get(vtepName)
	if err != nil {
		if !k8serror.IsNotFound(err) {
			return err
		}
		remoteVtep = utils.NewRemoteVtep(m.ClusterName, m.RemoteClusterUID, vtepIP, vtepMac, node.Name, nil)
		newVtep = true
	}
	curTime := metav1.Now()
	remoteVtep = remoteVtep.DeepCopy()

	remoteVtep.Spec.VtepIP = vtepIP
	remoteVtep.Spec.VtepMAC = vtepMac
	desired := utils.PickUsingIPList(instances)
	actual := remoteVtep.Spec.EndpointIPList
	desiredSet := gset.NewFrom(desired)
	actualSet := gset.NewFrom(actual)
	remove := actualSet.Diff(desiredSet)
	add := desiredSet.Diff(actualSet)

	remoteVtep.Spec.EndpointIPList = func() []string {
		ipList := make([]string, 0, desiredSet.Size())
		for _, v := range desiredSet.Slice() {
			ipList = append(ipList, fmt.Sprint(v))
		}
		return ipList
	}()
	vtepChanged := vtepIP != "" && vtepMac != "" && (vtepIP != remoteVtep.Spec.VtepIP || vtepMac != remoteVtep.Spec.VtepMAC)
	ipListChanged := remove.Size() != 0 || add.Size() != 0
	if !newVtep && !ipListChanged && !vtepChanged {
		return nil
	}

	if newVtep {
		remoteVtep, err = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().Create(context.TODO(), remoteVtep, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else {
		remoteVtep, err = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().Update(context.TODO(), remoteVtep, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	remoteVtep.Status.LastModifyTime = curTime
	_, err = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), remoteVtep, metav1.UpdateOptions{})
	return err
}

func (m *Manager) nodeToIPList(nodeName string) ([]string, error) {
	instances, err := m.IPLister.List(labels.SelectorFromSet(labels.Set{constants.LabelNode: nodeName}))
	if err != nil {
		return nil, err
	}
	return utils.PickUsingIPList(instances), nil
}

func (m *Manager) RunIPInstanceWorker() {
	for m.processNextIPInstance() {
	}
}

func (m *Manager) processNextIPInstance() bool {
	obj, shutdown := m.IPQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer m.IPQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			m.IPQueue.Forget(obj)
			return nil
		}
		if err := m.reconcileIPInstance(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			m.IPQueue.AddRateLimited(key)
			klog.Warningf("[RemoteCluster-IPInstance] failed to reconcileIPInstance. key=%v. err=%v", key, err)
			return fmt.Errorf("[RemoteCluster-IPInstance] fail to sync '%s' for cluster id=%v: %v, requeuing", key, m.ClusterName, err)
		}
		m.IPQueue.Forget(obj)
		klog.Infof("[RemoteCluster-IPInstance] succeed to sync '%s', cluster=%v", key, m.ClusterName)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}
	return true
}

func (m *Manager) filterIPInstance(obj interface{}) bool {
	if !m.GetIsReady() {
		return false
	}
	_, ok := obj.(*networkingv1.IPInstance)
	return ok
}

func (m *Manager) addOrDelIPInstance(obj interface{}) {
	ipInstance, _ := obj.(*networkingv1.IPInstance)
	m.enqueueIPInstance(ipInstance.Status.NodeName)
}

func (m *Manager) updateIPInstance(oldObj, newObj interface{}) {
	oldInstance, _ := oldObj.(*networkingv1.IPInstance)
	newInstance, _ := newObj.(*networkingv1.IPInstance)

	if oldInstance.ResourceVersion == newInstance.ResourceVersion {
		return
	}
	if newInstance.Status.Phase == networkingv1.IPPhaseReserved && oldInstance.Status.Phase == networkingv1.IPPhaseReserved {
		return
	}
	if newInstance.Status.NodeName != oldInstance.Status.NodeName {
		m.enqueueIPInstance(oldInstance.Status.NodeName)
		m.enqueueIPInstance(newInstance.Status.NodeName)
		return
	}

	if newInstance.Spec.Address.IP != oldInstance.Spec.Address.IP {
		m.enqueueIPInstance(newInstance.Status.NodeName)
	}
}

func (m *Manager) enqueueIPInstance(nodeName string) {
	m.IPQueue.Add(nodeName)
}
