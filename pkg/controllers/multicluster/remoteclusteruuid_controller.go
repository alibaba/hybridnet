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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

const ControllerRemoteClusterUUID = "RemoteClusterUUID"

type RemoteClusterUUIDReconciler struct {
	client.Client

	Recorder record.EventRecorder

	UUIDMutex UUIDMutex

	concurrency.ControllerConcurrency
}

func (r *RemoteClusterUUIDReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	var remoteCluster = &multiclusterv1.RemoteCluster{}
	if err = r.Get(ctx, types.NamespacedName{Name: req.Name}, remoteCluster); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch remote cluster", client.IgnoreNotFound(err))
	}

	if !remoteCluster.DeletionTimestamp.IsZero() {
		log.V(1).Info("remote cluster is terminating")
		return ctrl.Result{}, nil
	}

	var restConfig *rest.Config
	if restConfig, err = utils.NewRestConfigFromRemoteCluster(remoteCluster); err != nil {
		// clean uuid and set offline
		_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Status().Patch(ctx, remoteCluster, client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"status":{"uuid":"","state":%q,"conditions":null}}`, multiclusterv1.ClusterOffline))))
		})
		r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "InvalidRestConfig", err.Error())
		return ctrl.Result{}, wrapError("unable to get rest config", err)
	}

	var remoteClusterClient client.Client
	if remoteClusterClient, err = client.New(restConfig, client.Options{}); err != nil {
		// clean uuid and set offline
		_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Status().Patch(ctx, remoteCluster, client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"status":{"uuid":"","state":%q,"conditions":null}}`, multiclusterv1.ClusterOffline))))
		})
		r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "InvalidClusterClient", err.Error())
		return ctrl.Result{}, wrapError("unable to get cluster client", err)
	}

	var clusterUUID types.UID
	if clusterUUID, err = utils.GetClusterUUID(ctx, remoteClusterClient); err != nil {
		r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "GetUUIDFail", err.Error())
		return ctrl.Result{}, wrapError("unable to get UUID of remote cluster", err)
	}

	if remoteCluster.Status.UUID == clusterUUID {
		log.V(1).Info("remote cluster UUID is up-to-date")
		return ctrl.Result{}, nil
	}

	// try lock UUID to avoid conflict
	if _, err = r.UUIDMutex.Lock(clusterUUID, remoteCluster.Name); err != nil {
		r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "UUIDConflict", err.Error())
		return ctrl.Result{}, wrapError("unable to lock UUID for remote cluster", err)
	}

	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Status().Patch(ctx, remoteCluster, client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"status":{"uuid":%q}}`, clusterUUID))))
	}); err != nil {
		r.Recorder.Event(remoteCluster, corev1.EventTypeWarning, "UpdateUUIDFail", err.Error())
		return ctrl.Result{}, wrapError("unable to update uuid", err)
	}

	log.Info("update remote cluster UUID successfully", "UUID", clusterUUID)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteClusterUUIDReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerRemoteClusterUUID).
		For(&multiclusterv1.RemoteCluster{},
			builder.WithPredicates(
				&predicate.GenerationChangedPredicate{},
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
			RecoverPanic:            true,
		}).
		Complete(r)
}
