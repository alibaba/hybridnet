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
	"strings"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	"github.com/alibaba/hybridnet/pkg/utils"
)

type CheckStatusFunc func(c *Controller, hybridnetClient *versioned.Clientset, clusterName string) ([]networkingv1.ClusterCondition, error)

var (
	DefaultChecker    []CheckStatusFunc
	ConditionAllReady = make(map[networkingv1.ClusterConditionType]bool)
)

func init() {
	DefaultChecker = append(DefaultChecker, HealthChecker, BidirectionalConnChecker, OverlayNetIDChecker)

	ConditionAllReady[utils.TypeHealthCheck] = true
	ConditionAllReady[utils.TypeBidirectionalConn] = true
	ConditionAllReady[utils.TypeSameOverlayNetID] = true
}

func CheckCondition(c *Controller, hybridnetClient *versioned.Clientset, clusterName string,
	checkers []CheckStatusFunc) []networkingv1.ClusterCondition {
	conditions := make([]networkingv1.ClusterCondition, 0)
	for _, checker := range checkers {
		cond, err := checker(c, hybridnetClient, clusterName)
		if err != nil {
			break
		}
		conditions = append(conditions, cond...)
	}
	if meetCondition(conditions) {
		conditions = []networkingv1.ClusterCondition{utils.NewClusterReady()}
	}
	return conditions
}

func IsReady(conditions []networkingv1.ClusterCondition) bool {
	if len(conditions) == 1 {
		cond := conditions[0]
		return cond.Type == networkingv1.ClusterReady && cond.Status == apiv1.ConditionTrue
	}
	return false
}

func meetCondition(conditions []networkingv1.ClusterCondition) bool {
	cnt := 0
	for _, c := range conditions {
		if _, exists := ConditionAllReady[c.Type]; exists {
			if c.Status == apiv1.ConditionTrue {
				cnt = cnt + 1
			}
		}
	}
	return cnt == len(ConditionAllReady)
}

func HealthChecker(c *Controller, hybridnetClient *versioned.Clientset, clusterName string) ([]networkingv1.ClusterCondition, error) {
	conditions := make([]networkingv1.ClusterCondition, 0)

	body, err := hybridnetClient.Discovery().RESTClient().Get().AbsPath("/healthz").Do(context.TODO()).Raw()
	if err != nil {
		runtimeutil.HandleError(errors.Wrapf(err, "Cluster Health Check failed for cluster %v", clusterName))
		conditions = append(conditions, utils.NewClusterOffline(err))
		return conditions, err
	}
	if !strings.EqualFold(string(body), "ok") {
		conditions = append(conditions, utils.NewHealthCheckNotReady(err), utils.NewClusterNotOffline())
	} else {
		conditions = append(conditions, utils.NewHealthCheckReady())
	}
	return conditions, nil
}

// BidirectionalConnChecker check if remote cluster has create the remote cluster
func BidirectionalConnChecker(c *Controller, hybridnetClient *versioned.Clientset, clusterName string) ([]networkingv1.ClusterCondition, error) {
	conditions := make([]networkingv1.ClusterCondition, 0)

	rcs, err := hybridnetClient.NetworkingV1().RemoteClusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		runtimeutil.HandleError(err)
		conditions = append(conditions, utils.NewBidirectionalConnNotReady(err.Error()))
		return conditions, nil
	}
	doubleEndedConn := false
	for _, v := range rcs.Items {
		// has not set uuid, check next time
		if v.Status.UUID == "" {
			continue
		}
		if v.Status.UUID == c.UUID {
			doubleEndedConn = true
			break
		}
	}
	if !doubleEndedConn {
		klog.Warningf("The peer cluster has not created remote cluster. ClusterName=%v", clusterName)
		conditions = append(conditions, utils.NewBidirectionalConnNotReady(utils.MsgBidirectionalConnNotOk))
	} else {
		conditions = append(conditions, utils.NewBidirectionalConnReady())
	}
	return conditions, nil
}

func OverlayNetIDChecker(c *Controller, hybridnetClient *versioned.Clientset, clusterName string) ([]networkingv1.ClusterCondition, error) {
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("OverlayNetIDChecker panic. err=%v\n%v", err, debug.Stack())
		}
	}()
	conditions := make([]networkingv1.ClusterCondition, 0)

	networkList, err := hybridnetClient.NetworkingV1().Networks().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		runtimeutil.HandleError(err)
		conditions = append(conditions, utils.NewOverlayNetIDNotReady(err.Error()))
		return conditions, nil
	}
	var overlayNetID uint32
	for _, item := range networkList.Items {
		if item.Spec.Type == networkingv1.NetworkTypeOverlay && item.Spec.NetID != nil {
			overlayNetID = *item.Spec.NetID
			break
		}
	}
	c.overlayNetIDMU.RLock()
	defer c.overlayNetIDMU.RUnlock()

	if c.OverlayNetID == nil {
		conditions = append(conditions, utils.NewOverlayNetIDNotReady("local cluster has no overlay network"))
	} else if *c.OverlayNetID != overlayNetID {
		conditions = append(conditions, utils.NewOverlayNetIDNotReady("Different overlay net id"))
	} else {
		conditions = append(conditions, utils.NewOverlayNetIDReady())
	}
	return conditions, nil
}
