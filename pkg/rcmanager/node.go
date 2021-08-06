package rcmanager

import (
	"context"
	"fmt"
	"sync"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/utils"
	apiv1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
)

const ReconcileNode = "ReconcileNode"

// Full update. Update remote vtep expect status
func (m *Manager) reconcileNode() error {
	klog.Infof("[RemoteCluster] Starting reconcile node from cluster %v", m.ClusterName)
	nodes, err := m.nodeLister.List(labels.NewSelector())
	if err != nil {
		return err
	}
	vteps, err := m.remoteVtepLister.List(utils.SelectorClusterName(m.ClusterName))
	if err != nil {
		return err
	}

	add, update, remove := m.diffNodeAndVtep(nodes, vteps)
	var (
		wg  sync.WaitGroup
		cur = metav1.Now()
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		for _, v := range add {
			vtep, err := m.localClusterRamaClient.NetworkingV1().RemoteVteps().Create(context.TODO(), v, metav1.CreateOptions{})
			if err != nil {
				klog.Warningf("Can't create remote vtep in local cluster. err=%v. remote vtep name=%v", err, v.Name)
				continue
			}
			vtep.Status.LastModifyTime = cur
			_, err = m.localClusterRamaClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), vtep, metav1.UpdateOptions{})
			if err != nil {
				runtimeutil.HandleError(err)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for _, v := range update {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				vtep, err := m.localClusterRamaClient.NetworkingV1().RemoteVteps().Update(context.TODO(), v, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
				vtep.Status.LastModifyTime = cur
				_, err = m.localClusterRamaClient.NetworkingV1().RemoteVteps().UpdateStatus(context.TODO(), vtep, metav1.UpdateOptions{})
				return err
			})
			if err != nil {
				klog.Warningf("Can't update remote vtep in local cluster. err=%v. name=%v", err, v.Name)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for _, v := range remove {
			_ = m.localClusterRamaClient.NetworkingV1().RemoteVteps().Delete(context.TODO(), v, metav1.DeleteOptions{})
			if err != nil && !k8serror.IsNotFound(err) {
				klog.Warningf("Can't delete remote vtep in local cluster. remote vtep name=%v", v)
			}
		}
	}()
	wg.Wait()
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
		nodeIPList, err := m.nodeToIPList(node.Name)
		if err != nil {
			continue
		}
		vtepIP := node.Annotations[constants.AnnotationNodeVtepIP]
		vtepMac := node.Annotations[constants.AnnotationNodeVtepMac]
		if vtep, exists := vtepMap[vtepName]; exists {
			if vtep.Spec.VtepIP != vtepIP || vtep.Spec.VtepMAC != vtepMac {
				v := utils.NewRemoteVtep(m.ClusterName, m.RemoteClusterUID, vtepIP, vtepMac, node.Name, nodeIPList)
				update = append(update, v)
			}
		} else {
			v := utils.NewRemoteVtep(m.ClusterName, m.RemoteClusterUID, vtepIP, vtepMac, node.Name, nodeIPList)
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
	obj, shutdown := m.nodeQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer m.nodeQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			m.nodeQueue.Forget(obj)
			return nil
		}
		if err := m.reconcileNode(); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			m.nodeQueue.AddRateLimited(key)
			return fmt.Errorf("[RemoteCluster-Node] fail to sync '%s' for cluster=%v: %v, requeuing", key, m.ClusterName, err)
		}
		m.nodeQueue.Forget(obj)
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
	// todo debug
	klog.Infof("[RemoteCluster-Node]debug. node=%v", utils.ToJsonString(obj))
	_, ok := obj.(*apiv1.Node)
	return ok
}

func (m *Manager) addOrDelNode(obj interface{}) {
	node, _ := obj.(*apiv1.Node)
	m.EnqueueNode(node.Name)
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
		newNodeAnnotations[constants.AnnotationNodeVtepMac] == oldNodeAnnotations[constants.AnnotationNodeVtepMac] {
		return
	}
	m.EnqueueNode(newNode.Name)
}

func (m *Manager) EnqueueNode(nodeName string) {
	m.nodeQueue.Add(nodeName)
}
