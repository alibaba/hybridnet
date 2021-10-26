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
	"reflect"
	"sync"

	v1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"
	"k8s.io/utils/pointer"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controller/remotecluster/types"
	"github.com/alibaba/hybridnet/pkg/utils"
)

const (
	ReconcileSubnet = "ReconcileSubnet"
)

func (m *Manager) reconcileSubnet() error {
	klog.Infof("[remote cluster subnet] Starting reconcile subnet from cluster=%v", m.ClusterName)

	// list remote subnet of local cluster
	localClusterRemoteSubnets, err := m.RemoteSubnetLister.List(labels.Everything())
	if err != nil {
		return err
	}
	// list subnet of remote cluster
	remoteClusterSubnets, err := m.SubnetLister.List(labels.Everything())
	if err != nil {
		return err
	}
	// list network of remote cluster
	networks, err := m.NetworkLister.List(labels.Everything())
	if err != nil {
		return err
	}

	// transfer network slice into map
	networkMap := func() map[string]*networkingv1.Network {
		networkMap := make(map[string]*networkingv1.Network)
		for _, network := range networks {
			networkMap[network.Name] = network
		}
		return networkMap
	}()

	// diff expected subnets and existing subnets
	add, update, remove := m.diffSubnetAndRCSubnet(remoteClusterSubnets, localClusterRemoteSubnets, networkMap)

	// modify existing subnets to expectation
	var (
		wg      sync.WaitGroup
		errChan = make(chan error)
		errList []error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, toAdd := range add {
			rcSubnet, err := m.convertSubnet2RemoteSubnet(toAdd, networkMap[toAdd.Spec.Network])
			if err != nil {
				errChan <- fmt.Errorf("convert to remote subnet fail: %v, subnet=%v", err, toAdd.Name)
				continue
			}
			newSubnet, err := m.LocalClusterHybridnetClient.NetworkingV1().RemoteSubnets().Create(context.TODO(), rcSubnet, metav1.CreateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("create remote subnet fail: %v, subnet=%v", err, toAdd.Name)
				continue
			}
			newSubnet.Status.LastModifyTime = metav1.Now()
			_, err = m.LocalClusterHybridnetClient.NetworkingV1().RemoteSubnets().UpdateStatus(context.TODO(), newSubnet, metav1.UpdateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("update remote subnet status fail: %v, subnet=%v", err, toAdd.Name)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, toUpdate := range update {
			remoteSubnet, err := m.LocalClusterHybridnetClient.NetworkingV1().RemoteSubnets().Update(context.TODO(), toUpdate, metav1.UpdateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("update remote subnet fail: %v, subnet=%v", err, toUpdate.Name)
				continue
			}
			remoteSubnet.Status.LastModifyTime = metav1.Now()
			_, err = m.LocalClusterHybridnetClient.NetworkingV1().RemoteSubnets().UpdateStatus(context.TODO(), remoteSubnet, metav1.UpdateOptions{})
			if err != nil {
				errChan <- fmt.Errorf("update remote subnet status fail: %v, subnet=%v", err, toUpdate.Name)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, toRemove := range remove {
			_ = m.LocalClusterHybridnetClient.NetworkingV1().RemoteSubnets().Delete(context.TODO(), toRemove.Name, metav1.DeleteOptions{})
			if err != nil && !k8serror.IsNotFound(err) {
				errChan <- fmt.Errorf("remove remote subnet fail: %v, subnet=%v", err, toRemove.Name)
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
				EventType: v1.EventTypeWarning,
				Reason:    "SubnetReconciliationFail",
				Message:   aggregatedError.Error(),
			},
		}
		return fmt.Errorf("fail to reconcile remote subnets: %v, cluster=%v", aggregatedError, m.ClusterName)
	}

	m.eventHub <- types.Event{
		Type:        types.EventRecordEvent,
		ClusterName: m.ClusterName,
		Object: types.EventBody{
			EventType: v1.EventTypeNormal,
			Reason:    "SubnetReconciliationSucceed",
		},
	}
	return nil
}

func (m *Manager) RunSubnetWorker() {
	for m.processNextSubnet() {
	}
}

// Reconcile local cluster *networkingv1.RemoteSubnet based on remote cluster's subnet.
func (m *Manager) diffSubnetAndRCSubnet(subnets []*networkingv1.Subnet, rcSubnets []*networkingv1.RemoteSubnet,
	networkMap map[string]*networkingv1.Network) (
	add []*networkingv1.Subnet, update []*networkingv1.RemoteSubnet, remove []*networkingv1.RemoteSubnet) {
	subnetMap := func() map[string]*networkingv1.Subnet {
		subnetMap := make(map[string]*networkingv1.Subnet)
		for _, s := range subnets {
			subnetMap[s.Name] = s
		}
		return subnetMap
	}()

	for _, v := range rcSubnets {
		if v.Spec.ClusterName != m.ClusterName {
			continue
		}
		if subnet, exists := subnetMap[v.Labels[constants.LabelSubnet]]; !exists {
			remove = append(remove, v)
		} else {
			newestRemoteSubnet, err := m.convertSubnet2RemoteSubnet(subnet, networkMap[subnet.Spec.Network])
			if err != nil {
				continue
			}
			if !reflect.DeepEqual(newestRemoteSubnet.Spec, v.Spec) {
				update = append(update, newestRemoteSubnet)
			}
		}
	}
	remoteSubnetMap := func() map[string]*networkingv1.RemoteSubnet {
		remoteSubnetMap := make(map[string]*networkingv1.RemoteSubnet)
		for _, s := range rcSubnets {
			remoteSubnetMap[s.Name] = s
		}
		return remoteSubnetMap
	}()

	for _, s := range subnets {
		remoteSubnetName := utils.GenRemoteSubnetName(m.ClusterName, s.Name)
		if _, exists := remoteSubnetMap[remoteSubnetName]; !exists {
			add = append(add, s)
		}
	}
	return
}

func (m *Manager) convertSubnet2RemoteSubnet(subnet *networkingv1.Subnet, network *networkingv1.Network) (*networkingv1.RemoteSubnet, error) {
	if network == nil {
		return nil, fmt.Errorf("subnet corresponding Network is nil. Subnet=%v, Cluster=%v", subnet.Name, m.ClusterName)
	}
	rs := &networkingv1.RemoteSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.GenRemoteSubnetName(m.ClusterName, subnet.Name),
			Labels: map[string]string{
				constants.LabelCluster: m.ClusterName,
				constants.LabelSubnet:  subnet.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: networkingv1.SchemeGroupVersion.String(),
					Kind:       "RemoteCluster",
					Name:       m.ClusterName,
					UID:        m.RemoteClusterUID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: networkingv1.RemoteSubnetSpec{
			Range:       subnet.Spec.Range,
			Type:        network.Spec.Type,
			ClusterName: m.ClusterName,
		},
	}
	return rs, nil
}

func (m *Manager) filterSubnet(obj interface{}) bool {
	if !m.GetIsReady() {
		return false
	}
	_, ok := obj.(*networkingv1.Subnet)
	return ok
}

func (m *Manager) addOrDelSubnet(_ interface{}) {
	m.EnqueueSubnet(ReconcileSubnet)
}

func (m *Manager) updateSubnet(oldObj, newObj interface{}) {
	oldRC, _ := oldObj.(*networkingv1.Subnet)
	newRC, _ := newObj.(*networkingv1.Subnet)

	if oldRC.ResourceVersion == newRC.ResourceVersion {
		return
	}
	m.EnqueueSubnet(ReconcileSubnet)
}

func (m *Manager) EnqueueSubnet(subnetName string) {
	m.SubnetQueue.Add(subnetName)
}

func (m *Manager) processNextSubnet() bool {
	obj, shutdown := m.SubnetQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer m.SubnetQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			m.SubnetQueue.Forget(obj)
			return nil
		}
		if err := m.reconcileSubnet(); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			m.SubnetQueue.AddRateLimited(key)
			return fmt.Errorf("[remote cluster subnet] fail to sync subnet for cluster=%v: %v, requeuing", m.ClusterName, err)
		}
		m.SubnetQueue.Forget(obj)
		klog.Infof("[remote cluster subnet] succeed to sync subnet for cluster=%v", m.ClusterName)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}
