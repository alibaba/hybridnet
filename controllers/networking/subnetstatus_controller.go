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

package networking

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
	"github.com/alibaba/hybridnet/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
)

// SubnetStatusReconciler reconciles a Subnet object
type SubnetStatusReconciler struct {
	client.Client

	IPAMManager IPAMManager
	Recorder    record.EventRecorder
}

//+kubebuilder:rbac:groups=networking.alibaba.com,resources=subnets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=subnets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=subnets/finalizers,verbs=update

func (r *SubnetStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	var subnet = &networkingv1.Subnet{}

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
			if len(subnet.UID) > 0 {
				r.Recorder.Event(subnet, corev1.EventTypeWarning, "UpdateStatusFail", err.Error())
			}
		}
	}()

	if err = r.Get(ctx, req.NamespacedName, subnet); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch Subnet", client.IgnoreNotFound(err))
	}

	// fetch subnet usage from manager
	var usage *ipamtypes.Usage
	if feature.DualStackEnabled() {
		if usage, err = r.IPAMManager.DualStack().SubnetUsage(subnet.Spec.Network, subnet.Name); err != nil {
			return ctrl.Result{}, wrapError("unable to fetch subnet usage", err)
		}
	} else {
		if usage, err = r.IPAMManager.SubnetUsage(subnet.Spec.Network, subnet.Name); err != nil {
			return ctrl.Result{}, wrapError("unable to fetch subnet usage", err)
		}
	}

	var subnetStatus = &networkingv1.SubnetStatus{
		Count: networkingv1.Count{
			Total:     int32(usage.Total),
			Used:      int32(usage.Used),
			Available: int32(usage.Available),
		},
		LastAllocatedIP: usage.LastAllocation,
	}

	// diff for no-op
	if reflect.DeepEqual(&subnet.Status, subnetStatus) {
		log.V(10).Info("subnet status is up-to-date, skip updating")
		return ctrl.Result{}, nil
	}

	// patch subnet status
	subnetPatch := client.MergeFrom(subnet.DeepCopy())
	subnet.Status = *subnetStatus
	if err = retry.RetryOnConflict(retry.DefaultRetry,
		func() error {
			return r.Status().Patch(ctx, subnet, subnetPatch)
		},
	); err != nil {
		return ctrl.Result{}, wrapError("unable to update subnet status", err)
	}

	log.V(8).Info(fmt.Sprintf("sync subnet status to %+v", subnetStatus))
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SubnetStatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Subnet{},
			builder.WithPredicates(
				&predicate.GenerationChangedPredicate{},
				&utils.IgnoreDeletePredicate{},
				&utils.SubnetSpecChangePredicate{},
			)).
		Watches(&source.Kind{Type: &networkingv1.IPInstance{}},
			handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
				ipInstance, ok := object.(*networkingv1.IPInstance)
				if !ok {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name: ipInstance.Spec.Subnet,
						},
					},
				}
			}),
			builder.WithPredicates(
				&utils.IgnoreUpdatePredicate{},
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			Log:                     mgr.GetLogger().WithName("SubnetStatusController"),
		}).
		Complete(r)
}
