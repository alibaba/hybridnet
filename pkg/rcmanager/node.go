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
	"errors"
	"fmt"
	"sync"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/utils"
	apiv1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

const ReconcileNode = "ReconcileNode"

// Full update. Update remote vtep expect status
func (m *Manager) reconcileNode() error {
	klog.Infof("[RemoteCluster] Starting reconcile node from cluster %v", m.ClusterName)
	nodes, err := m.NodeLister.List(labels.Everything())
	if err != nil {
		return err
	}
	vteps, err := m.RemoteVtepLister.List(utils.SelectorClusterName(m.ClusterName))
	if err != nil {
		return err
	}

	add, update, remove := m.diffNodeAndVtep(nodes, vteps)
	var (
		wg        sync.WaitGroup
		cur       = metav1.Now()
		errHappen bool
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, v := range add {
			vtep, err := m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().Create(context.TODO(), v, metav1.CreateOptions{})
			if err != nil {
				errHappen = true
				klog.Warningf("Can't create remote vtep in local cluster. err=%v. remote vtep name=%v", err, v.Name)
				continue
			}
			vtep.Status.LastModifyTime = cur
			_, err = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), vtep, metav1.UpdateOptions{})
			if err != nil {
				errHappen = true
				klog.Warningf("Can't update remote vtep status in local cluster. err=%v. remote vtep name=%v", err, v.Name)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, v := range update {
			vtep, err := m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().Update(context.TODO(), v, metav1.UpdateOptions{})
			if err != nil {
				errHappen = true
				klog.Warningf("Can't update remote vtep in local cluster. err=%v. name=%v", err, v.Name)
				continue
			}
			vtep.Status.LastModifyTime = cur
			_, err = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), vtep, metav1.UpdateOptions{})
			if err != nil {
				errHappen = true
				klog.Warningf("Can't update remote vtep status in local cluster. err=%v. name=%v", err, v.Name)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, v := range remove {
			_ = m.LocalClusterRamaClient.NetworkingV1().RemoteVteps().Delete(context.TODO(), v, metav1.DeleteOptions{})
			if err != nil && !k8serror.IsNotFound(err) {
				errHappen = true
				klog.Warningf("Can't delete remote vtep in local cluster. remote vtep name=%v", v)
			}
		}
	}()

	wg.Wait()
	if errHappen {
		return errors.New("some error happened in add/update/remove function")
	}
	return nil
}

func (m *Manager) diffNodeAndVtep(nodes []*apiv1.Node, vteps []*networkingv1.RemoteVtep) (
	add []*networkingv1.RemoteVtep, update []*networkingv1.RemoteVtep, remove []string) {
	nodeMap := func() map[string]*apiv1.Node {
		nodeMap := make(map[string]*apiv1.Node)
		for _, node := range nodes {
			nodeMap[node.Name] = node
		}
		return nodeMap
	}()
	vtepMap := func() map[string]*networkingv1.RemoteVtep {
		vtepMap := make(map[string]*networkingv1.RemoteVtep)
		for _, vtep := range vteps {
			vtepMap[vtep.Name] = vtep
		}
		return vtepMap
	}()

	for _, node := range nodes {
		vtepName := utils.GenRemoteVtepName(m.ClusterName, node.Name)
		if vtepName == "" {
			continue
		}
		if vtep, exists := vtepMap[vtepName]; exists {
			remoteVtepChanged, newRemoteVtep, err := m.RemoteVtepChanged(vtep, node)
			if err != nil {
				continue
			}
			if remoteVtepChanged {
				update = append(update, newRemoteVtep)
			}
		} else {
			endpointIPList, err := m.pickEndpointListFromNode(node)
			if err != nil {
				continue
			}
			v := utils.NewRemoteVtep(m.ClusterName, m.RemoteClusterUID, node.Annotations[constants.AnnotationNodeVtepIP], node.Annotations[constants.AnnotationNodeVtepMac],
				node.Annotations[constants.AnnotationNodeLocalVxlanIPList], node.Name, endpointIPList)
			add = append(add, v)
		}

	}
	for _, vtep := range vteps {
		if _, exists := nodeMap[vtep.Spec.NodeName]; !exists {
			remove = append(remove, vtep.Name)
		}
	}
	return
}

func (m *Manager) processNextNode() bool {
	obj, shutdown := m.NodeQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer m.NodeQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			m.NodeQueue.Forget(obj)
			return nil
		}
		if err := m.reconcileNode(); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			m.NodeQueue.AddRateLimited(key)
			return fmt.Errorf("[RemoteCluster-Node] fail to sync '%s' for cluster=%v: %v, requeuing", key, m.ClusterName, err)
		}
		m.NodeQueue.Forget(obj)
		klog.Infof("[RemoteCluster-Node] succeed to sync '%s', cluster=%v", key, m.ClusterName)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true

}

func (m *Manager) RunNodeWorker() {
	for m.processNextNode() {
	}
}

func (m *Manager) filterNode(obj interface{}) bool {
	if !m.GetIsReady() {
		return false
	}
	_, ok := obj.(*apiv1.Node)
	return ok
}

func (m *Manager) addOrDelNode(_ interface{}) {
	m.EnqueueNode(ReconcileNode)
}

func (m *Manager) updateNode(oldObj, newObj interface{}) {
	oldNode, _ := oldObj.(*apiv1.Node)
	newNode, _ := newObj.(*apiv1.Node)
	newNodeAnnotations := newNode.Annotations
	oldNodeAnnotations := oldNode.Annotations

	if newNodeAnnotations[constants.AnnotationNodeVtepIP] == "" || newNodeAnnotations[constants.AnnotationNodeVtepMac] == "" {
		return
	}
	if newNodeAnnotations[constants.AnnotationNodeVtepIP] == oldNodeAnnotations[constants.AnnotationNodeVtepIP] &&
		newNodeAnnotations[constants.AnnotationNodeVtepMac] == oldNodeAnnotations[constants.AnnotationNodeVtepMac] &&
		newNodeAnnotations[constants.AnnotationNodeLocalVxlanIPList] == oldNodeAnnotations[constants.AnnotationNodeLocalVxlanIPList] {
		return
	}
	m.EnqueueNode(ReconcileNode)
}

func (m *Manager) EnqueueNode(nodeName string) {
	m.NodeQueue.Add(nodeName)
}
