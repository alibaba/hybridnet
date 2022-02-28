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

package multicluster

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/multicluster/clusterchecker"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/managerruntime"
	"github.com/alibaba/hybridnet/pkg/metrics"
)

const CheckerRemoteClusterStatus = "RemoteClusterStatus"

const (
	ConditionDaemonRegistered = "DaemonRegistered"
	ConditionCheckerExecuted  = "CheckerExecuted"
)

type RemoteClusterStatusChecker struct {
	client.Client
	Logger logr.Logger

	CheckPeriod time.Duration

	DaemonHub managerruntime.DaemonHub

	Checker clusterchecker.Checker

	Event <-chan ClusterCheckEvent

	Recorder record.EventRecorder
}

func (r *RemoteClusterStatusChecker) Start(ctx context.Context) error {
	r.Logger.Info("remote cluster status checker is starting")

	ticker := time.NewTicker(r.CheckPeriod)

	for {
		select {
		case <-ticker.C:
			r.Logger.V(1).Info("cron job for all clusters")
			r.crontab(ctx)
		case event := <-r.Event:
			r.Logger.Info("single job for one cluster registration")
			r.checkClusterStatus(event.Context, event.Name, event.DaemonID)
		case <-ctx.Done():
			ticker.Stop()
			r.Logger.Info("remote cluster status checker is stopping")
			return nil
		}
	}
}

func (r *RemoteClusterStatusChecker) crontab(ctx context.Context) {
	defer utilruntime.HandleCrash()

	remoteClusterList, err := utils.ListRemoteClusters(r)
	if err != nil {
		r.Logger.Error(err, "unable to fetch remote clusters")
		return
	}

	nameDaemonIDMap := make(map[string]managerruntime.DaemonID)

	for i := range remoteClusterList.Items {
		var remoteCluster = &remoteClusterList.Items[i]
		if len(remoteCluster.Status.UUID) > 0 {
			nameDaemonIDMap[remoteCluster.Name] = managerruntime.DaemonID(remoteCluster.Status.UUID)
		}
	}

	r.Logger.V(1).Info("check remote cluster status periodically", "clusters", nameDaemonIDMap)

	for name, id := range nameDaemonIDMap {
		r.checkClusterStatus(ctx, name, id)
	}

}

func (r *RemoteClusterStatusChecker) checkClusterStatus(ctx context.Context, name string, daemonID managerruntime.DaemonID) {
	start := time.Now()
	defer func() {
		metrics.RemoteClusterStatusCheckDuration.WithLabelValues(name).Observe(time.Since(start).Seconds())
	}()

	remoteCluster, err := utils.GetRemoteCluster(r, name)
	if err != nil {
		// TODO: handle fetch error here
		return
	}

	_, err = controllerutil.CreateOrPatch(ctx, r, remoteCluster, func() (err error) {
		var managerRuntime managerruntime.ManagerRuntime
		if managerRuntime, err = r.getManagerRuntimeByDaemonID(daemonID); err != nil {
			remoteCluster.Status.State = multiclusterv1.ClusterOffline
			fillCondition(&remoteCluster.Status, &metav1.Condition{
				Type:               ConditionDaemonRegistered,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: remoteCluster.Generation,
				LastTransitionTime: metav1.Now(),
				Reason:             "NotFound",
				Message:            err.Error(),
			})
			return nil
		}

		fillCondition(&remoteCluster.Status, &metav1.Condition{
			Type:               ConditionDaemonRegistered,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: remoteCluster.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "Registered",
		})

		defer func() {
			// TODO: more cases
			switch remoteCluster.Status.State {
			case multiclusterv1.ClusterReady:
				if !managerRuntime.Status().Running() {
					if err = r.DaemonHub.Run(daemonID); err != nil {
						r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "RunDaemonFail", err.Error())
					}
					err = wrapError("unable to run cluster daemon", err)
				}
			case multiclusterv1.ClusterNotReady:
				if managerRuntime.Status().Running() {
					if err = r.DaemonHub.Stop(daemonID); err != nil {
						r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "StopDaemonFail", err.Error())
					}
					err = wrapError("unable to stop cluster daemon", err)
				}
			}
		}()

		results, err := r.Checker.CheckAll(managerRuntime.Manager(), clusterchecker.ClusterName(name))
		if err != nil {
			remoteCluster.Status.State = multiclusterv1.ClusterNotReady
			fillCondition(&remoteCluster.Status, &metav1.Condition{
				Type:               ConditionCheckerExecuted,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: remoteCluster.Generation,
				LastTransitionTime: metav1.Now(),
				Reason:             "CheckerRunFail",
				Message:            err.Error(),
			})
			return nil
		}

		fillCondition(&remoteCluster.Status, &metav1.Condition{
			Type:               ConditionCheckerExecuted,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: remoteCluster.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "CheckerRunSucceed",
		})

		var allCheckPass = true
		for checkName, result := range results {
			condition := &metav1.Condition{
				Type:               checkName,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: remoteCluster.Generation,
				LastTransitionTime: metav1.Time{Time: result.TimeStamp()},
				Reason:             "CheckPass",
			}

			if !result.Succeed() {
				allCheckPass = false
				condition.Status = metav1.ConditionFalse
				condition.Reason = "CheckFail"
				condition.Message = result.Error().Error()
			}

			fillCondition(&remoteCluster.Status, condition)
		}

		if allCheckPass {
			remoteCluster.Status.State = multiclusterv1.ClusterReady
		} else {
			remoteCluster.Status.State = multiclusterv1.ClusterNotReady
		}
		return nil
	})

	if err != nil {
		r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "CheckStatusFail", err.Error())
		r.Logger.Error(err, "unable to check cluster status", "RemoteCluster", name)
	}
}

func (r *RemoteClusterStatusChecker) getManagerRuntimeByDaemonID(daemonID managerruntime.DaemonID) (managerruntime.ManagerRuntime, error) {
	d, found := r.DaemonHub.Get(daemonID)
	if !found {
		return nil, fmt.Errorf("daemon %s not registered in hub", daemonID)
	}

	mr, ok := d.(managerruntime.ManagerRuntime)
	if !ok {
		return nil, fmt.Errorf("daemon %s can not case to manager runtime", daemonID)
	}
	return mr, nil
}

func fillCondition(status *multiclusterv1.RemoteClusterStatus, condition *metav1.Condition) {
	if len(status.Conditions) == 0 {
		status.Conditions = []metav1.Condition{
			*condition,
		}
	}

	idx := -1
	for i := range status.Conditions {
		if status.Conditions[i].Type == condition.Type {
			idx = i
			break
		}
	}

	if idx < 0 {
		status.Conditions = append(status.Conditions, *condition)
	} else {
		status.Conditions[idx] = *condition
	}
}
