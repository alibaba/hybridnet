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
	"sync"

	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controller/remotecluster/types"
	"github.com/alibaba/hybridnet/pkg/utils"
)

const ReconcileNode = "ReconcileNode"

// Full update. Update remote vtep expect status
func (m *Manager) reconcileNode() error {
	klog.Infof("[remote cluster node] Starting reconcile nodes from cluster=%v", m.ClusterName)

	// list node of remote cluster
	nodes, err := m.NodeLister.List(labels.Everything())
	if err != nil {
		return err
	}

	// list remote vtep of local cluster
	vteps, err := m.RemoteVtepLister.List(utils.SelectorClusterName(m.ClusterName))
	if err != nil {
		return err
	}

	// diff
	add, update, remove := m.diffNodeAndVtep(nodes, vteps)

	// actions
	var (
		wg      sync.WaitGroup
		errChan = make(chan error)
		errList []error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, toAdd := range add {
			vtep, err := m.LocalClusterHybridnetClient.NetworkingV1().RemoteVteps().Create(context.TODO(), toAdd, metav1.CreateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("create remote vtep fail: %v, vtep=%v", err, toAdd.Name)
				continue
			}
			vtep.Status.LastModifyTime = metav1.Now()
			_, err = m.LocalClusterHybridnetClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), vtep, metav1.UpdateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("update remote vtep status fail: %v, vtep=%v", err, toAdd.Name)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, toUpdate := range update {
			vtep, err := m.LocalClusterHybridnetClient.NetworkingV1().RemoteVteps().Update(context.TODO(), toUpdate, metav1.UpdateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("update remote vtep fail: %v, vtep=%v", err, toUpdate.Name)
				continue
			}
			vtep.Status.LastModifyTime = metav1.Now()
			_, err = m.LocalClusterHybridnetClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), vtep, metav1.UpdateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("update remote vtep status fail: %v, vtep=%v", err, toUpdate.Name)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, toRemove := range remove {
			_ = m.LocalClusterHybridnetClient.NetworkingV1().RemoteVteps().Delete(context.TODO(), toRemove, metav1.DeleteOptions{})
			if err != nil && !k8serror.IsNotFound(err) {
				errChan <- fmt.Errorf("remove remote vtep fail: %v, vtep=%v", err, toRemove)
			}
		}
	}()

	go func() {
		for err := range errChan {
			errList = append(errList, err)
		}
	}()

	wg.Wait()
	close(errChan)

	if aggregatedError := errors.NewAggregate(errList); aggregatedError != nil {
		m.eventHub <- types.Event{
			Type:        types.EventRecordEvent,
			ClusterName: m.ClusterName,
			Object: types.EventBody{
				EventType: corev1.EventTypeWarning,
				Reason:    "NodeReconciliationFail",
				Message:   aggregatedError.Error(),
			},
		}
		return fmt.Errorf("fail to reconcile remote nodes: %v, cluster=%v", aggregatedError, m.ClusterName)
	}

	m.eventHub <- types.Event{
		Type:        types.EventRecordEvent,
		ClusterName: m.ClusterName,
		Object: types.EventBody{
			EventType: corev1.EventTypeNormal,
			Reason:    "NodeReconciliationSucceed",
		},
	}
	return nil
}

func (m *Manager) diffNodeAndVtep(nodes []*corev1.Node, vteps []*networkingv1.RemoteVtep) (add []*networkingv1.RemoteVtep, update []*networkingv1.RemoteVtep, remove []string) {
	nodeMap := func() map[string]*corev1.Node {
		nodeMap := make(map[string]*corev1.Node)
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
			return fmt.Errorf("[remote cluster node] fail to sync nodes for cluster=%v: %v, requeuing", m.ClusterName, err)
		}
		m.NodeQueue.Forget(obj)
		klog.Infof("[remote cluster node] succeed to sync nodes for cluster=%v", m.ClusterName)
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
	_, ok := obj.(*corev1.Node)
	return ok
}

func (m *Manager) addOrDelNode(_ interface{}) {
	m.EnqueueAllNode()
}

func (m *Manager) updateNode(oldObj, newObj interface{}) {
	oldNode, _ := oldObj.(*corev1.Node)
	newNode, _ := newObj.(*corev1.Node)
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
	m.EnqueueAllNode()
}

func (m *Manager) EnqueueAllNode() {
	m.NodeQueue.Add(ReconcileNode)
}
