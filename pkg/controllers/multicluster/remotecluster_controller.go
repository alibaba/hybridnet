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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/managerruntime"
)

const ControllerRemoteCluster = "RemoteCluster"

// RemoteClusterReconciler reconciles a RemoteCluster object
type RemoteClusterReconciler struct {
	client.Client

	Recorder record.EventRecorder

	UUIDMutex UUIDMutex

	DaemonHub managerruntime.DaemonHub

	Event chan<- ClusterCheckEvent

	LocalManager manager.Manager

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remoteclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remoteclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remoteclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RemoteCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *RemoteClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	var remoteCluster = &multiclusterv1.RemoteCluster{}
	if err = r.Get(ctx, req.NamespacedName, remoteCluster); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch remote cluster", client.IgnoreNotFound(err))
	}

	if !remoteCluster.DeletionTimestamp.IsZero() || len(remoteCluster.Status.UUID) == 0 {
		// recycle all orphan UUIDs of this remote cluster
		for _, orphanUUID := range r.UUIDMutex.GetUUIDs(remoteCluster.Name) {
			orphanDaemonID := managerruntime.DaemonID(orphanUUID)
			if err = r.killDaemon(ctx, orphanDaemonID); err != nil {
				return ctrl.Result{}, wrapError("unable to kill daemon", err)
			}
			_ = r.UUIDMutex.Unlock(orphanUUID)
		}
		if err = r.removeFinalizer(ctx, remoteCluster); err != nil {
			return ctrl.Result{}, wrapError("unable to remove finalizer", err)
		}
		return ctrl.Result{}, nil
	}

	// check whether this UUID is matched and latest
	var latestUUIDExisting bool
	var latestUUID types.UID
	latestUUIDExisting, latestUUID = r.UUIDMutex.GetLatestUUID(remoteCluster.Name)
	if !latestUUIDExisting {
		return ctrl.Result{}, fmt.Errorf("uuid is not locked by remote cluster %s", remoteCluster.Name)
	}
	if latestUUID != remoteCluster.Status.UUID {
		return ctrl.Result{}, fmt.Errorf("uuid locked by remote cluster %s is out-of-date", remoteCluster.Name)
	}

	// check and recycle orphan UUIDs of this remote cluster
	for _, orphanUUID := range r.UUIDMutex.GetUUIDs(remoteCluster.Name) {
		if orphanUUID == latestUUID {
			orphanDaemonID := managerruntime.DaemonID(orphanUUID)
			if err = r.killDaemon(ctx, orphanDaemonID); err != nil {
				return ctrl.Result{}, wrapError("unable to kill daemon", err)
			}
			_ = r.UUIDMutex.Unlock(orphanUUID)
		}
	}

	// generate rest config and manager runtime
	var restConfig *rest.Config
	if restConfig, err = utils.NewRestConfigFromRemoteCluster(remoteCluster); err != nil {
		return ctrl.Result{}, wrapError("unable to get rest config", err)
	}
	var managerRuntime managerruntime.ManagerRuntime
	if managerRuntime, err = r.constructClusterManagerRuntime(req.Name, restConfig); err != nil {
		return ctrl.Result{}, wrapError("unable to create manager runtime", err)
	}

	// add finalizer
	if err = r.addFinalizer(ctx, remoteCluster); err != nil {
		return ctrl.Result{}, wrapError("unable to add finalzier", err)
	}

	// guard manager runtime as daemon
	if err = r.guardDaemon(ctx, req.Name, managerruntime.DaemonID(latestUUID), managerRuntime); err != nil {
		return ctrl.Result{}, wrapError("unable to guard mr daemon", err)
	}

	log.V(4).Info("create and guard manager runtime successfully")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.LocalManager = mgr
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerRemoteCluster).
		For(&multiclusterv1.RemoteCluster{},
			builder.WithPredicates(
				&utils.IgnoreDeletePredicate{},
				// TODO: spec-hash change predicate
				&utils.RemoteClusterUUIDChangePredicate{},
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
		}).
		Complete(r)
}

func (r *RemoteClusterReconciler) killDaemon(ctx context.Context, daemonID managerruntime.DaemonID) (err error) {
	if !r.DaemonHub.IsRegistered(daemonID) {
		return nil
	}
	if err = r.DaemonHub.Stop(daemonID); err != nil {
		return wrapError("unable to stop mr daemon", err)
	}
	if err = r.DaemonHub.Unregister(daemonID); err != nil {
		return wrapError("unable to unregister mr daemon", err)
	}
	return nil
}

func (r *RemoteClusterReconciler) guardDaemon(ctx context.Context, name string, daemonID managerruntime.DaemonID, daemon managerruntime.Daemon) (err error) {
	// FIXME: use cluster spec-hash to make restart reasonable
	if err = r.killDaemon(ctx, daemonID); err != nil {
		return err
	}
	if err = r.DaemonHub.Register(daemonID, daemon); err != nil {
		return wrapError("unable to register mr daemon", err)
	}

	// event checker to check and run this cluster
	r.Event <- ClusterCheckEvent{
		Context:  ctx,
		Name:     name,
		DaemonID: daemonID,
	}
	return nil
}

func (r *RemoteClusterReconciler) constructClusterManagerRuntime(name string, restConfig *rest.Config) (managerruntime.ManagerRuntime, error) {
	managerRuntime, err := managerruntime.NewManagerRuntime(name,
		restConfig,
		&manager.Options{
			Scheme: r.LocalManager.GetScheme(),
			Logger: r.LocalManager.GetLogger().WithName("manager-runtime").WithName(name),
		},
	)
	if err != nil {
		return nil, err
	}

	// inject RemoteSubnetReconciler
	if err = (&RemoteSubnetReconciler{
		Client:        managerRuntime.GetClient(),
		ClusterName:   name,
		ParentCluster: r.LocalManager,
	}).SetupWithManager(managerRuntime); err != nil {
		return nil, wrapError("unable to inject remote subnet reconciler", err)
	}

	// inject RemoteVtepReconciler
	if err = (&RemoteVtepReconciler{
		Client:        managerRuntime.GetClient(),
		ClusterName:   name,
		ParentCluster: r.LocalManager,
	}).SetupWithManager(managerRuntime); err != nil {
		return nil, wrapError("unable to inject remote VTEP reconciler", err)
	}

	return managerRuntime, nil
}

func (r *RemoteClusterReconciler) addFinalizer(ctx context.Context, remoteCluster *multiclusterv1.RemoteCluster) error {
	if controllerutil.ContainsFinalizer(remoteCluster, constants.FinalizerManagerRuntimeRegistered) {
		return nil
	}

	patch := client.MergeFrom(remoteCluster.DeepCopy())
	controllerutil.AddFinalizer(remoteCluster, constants.FinalizerManagerRuntimeRegistered)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, remoteCluster, patch)
	})
}

func (r *RemoteClusterReconciler) removeFinalizer(ctx context.Context, remoteCluster *multiclusterv1.RemoteCluster) error {
	if !controllerutil.ContainsFinalizer(remoteCluster, constants.FinalizerManagerRuntimeRegistered) {
		return nil
	}

	patch := client.MergeFrom(remoteCluster.DeepCopy())
	controllerutil.RemoveFinalizer(remoteCluster, constants.FinalizerManagerRuntimeRegistered)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, remoteCluster, patch)
	})
}
