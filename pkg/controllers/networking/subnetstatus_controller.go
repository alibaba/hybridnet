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

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/metrics"
)

const ControllerSubnetStatus = "SubnetStatus"

// SubnetStatusReconciler reconciles a Subnet object
type SubnetStatusReconciler struct {
	client.Client

	IPAMManager IPAMManager
	Recorder    record.EventRecorder

	SubnetStatusUpdateChan <-chan event.GenericEvent

	concurrency.ControllerConcurrency
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

	if subnet.DeletionTimestamp != nil {
		cleanSubnetMetrics(subnet.Spec.Network, subnet.Name)
		return ctrl.Result{}, wrapError("unable to remove finalizer", r.removeFinalizer(ctx, subnet))

	}

	// make sure metrics will be un-registered before deletion
	if err = r.addFinalizer(ctx, subnet); err != nil {
		return ctrl.Result{}, wrapError("unable to add finalizer to subnet", err)
	}

	// fetch subnet usage from manager
	var usage *ipamtypes.Usage
	if usage, err = r.IPAMManager.GetSubnetUsage(subnet.Spec.Network, subnet.Name); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch subnet usage", err)
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
		log.V(1).Info("subnet status is up-to-date, skip updating")
		return ctrl.Result{}, nil
	}

	// update metrics
	updateSubnetUsageMetrics(subnet.Spec.Network, subnet.Name, &subnet.Status)

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

	log.V(1).Info(fmt.Sprintf("sync subnet status to %+v", subnetStatus))
	return ctrl.Result{}, nil
}

func updateSubnetUsageMetrics(networkName, subnetName string, subnetStatus *networkingv1.SubnetStatus) {
	if subnetStatus.Total > 0 {
		metrics.SubnetIPUsageGauge.With(
			prometheus.Labels{
				"subnetName":  subnetName,
				"networkName": networkName,
				"usageType":   metrics.IPTotalUsageType,
			},
		).Set(float64(subnetStatus.Total))

		metrics.SubnetIPUsageGauge.With(
			prometheus.Labels{
				"subnetName":  subnetName,
				"networkName": networkName,
				"usageType":   metrics.IPUsedUsageType,
			},
		).Set(float64(subnetStatus.Used))

		metrics.SubnetIPUsageGauge.With(
			prometheus.Labels{
				"subnetName":  subnetName,
				"networkName": networkName,
				"usageType":   metrics.IPAvailableUsageType,
			},
		).Set(float64(subnetStatus.Available))
	}
}

func cleanSubnetMetrics(networkName, subnetName string) {
	_ = metrics.SubnetIPUsageGauge.Delete(
		prometheus.Labels{
			"subnetName":  subnetName,
			"networkName": networkName,
			"usageType":   metrics.IPTotalUsageType,
		})

	_ = metrics.SubnetIPUsageGauge.Delete(
		prometheus.Labels{
			"subnetName":  subnetName,
			"networkName": networkName,
			"usageType":   metrics.IPUsedUsageType,
		})

	_ = metrics.SubnetIPUsageGauge.Delete(
		prometheus.Labels{
			"subnetName":  subnetName,
			"networkName": networkName,
			"usageType":   metrics.IPAvailableUsageType,
		})
}

func (r *SubnetStatusReconciler) addFinalizer(ctx context.Context, subnet *networkingv1.Subnet) error {
	if controllerutil.ContainsFinalizer(subnet, constants.FinalizerMetricsRegistered) {
		return nil
	}

	patch := client.MergeFrom(subnet.DeepCopy())
	controllerutil.AddFinalizer(subnet, constants.FinalizerMetricsRegistered)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, subnet, patch)
	})
}

func (r *SubnetStatusReconciler) removeFinalizer(ctx context.Context, subnet *networkingv1.Subnet) error {
	if !controllerutil.ContainsFinalizer(subnet, constants.FinalizerMetricsRegistered) {
		return nil
	}

	patch := client.MergeFrom(subnet.DeepCopy())
	controllerutil.RemoveFinalizer(subnet, constants.FinalizerMetricsRegistered)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, subnet, patch)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *SubnetStatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerSubnetStatus).
		For(&networkingv1.Subnet{},
			builder.WithPredicates(
				&utils.IgnoreDeletePredicate{},
				&predicate.ResourceVersionChangedPredicate{},
				predicate.Or(
					&utils.SubnetSpecChangePredicate{},
					&utils.TerminatingPredicate{},
				),
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
		Watches(&source.Channel{Source: r.SubnetStatusUpdateChan, DestBufferSize: 100},
			&handler.EnqueueRequestForObject{},
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
			RecoverPanic:            true,
		}).
		Complete(r)
}
