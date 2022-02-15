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

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/ipam"
)

const ControllerIPAM = "IPAM"

// IPAMReconciler reconciles IPAM Manager
type IPAMReconciler struct {
	client.Client

	Refresh ipam.Refresh

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks/finalizers,verbs=update

func (r *IPAMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	if err := r.Refresh.Refresh([]string{req.Name}); err != nil {
		log.Error(err, "unable to refresh IPAM Manager", "network", req.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Network{},
			builder.WithPredicates(
				&predicate.GenerationChangedPredicate{},
				&utils.NetworkSpecChangePredicate{},
			)).
		Watches(&source.Kind{Type: &networkingv1.Subnet{}},
			handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
				subnet, ok := object.(*networkingv1.Subnet)
				if !ok {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name: subnet.Spec.Network,
						},
					},
				}
			}),
			builder.WithPredicates(
				&predicate.GenerationChangedPredicate{},
				&utils.SubnetSpecChangePredicate{},
			)).
		WithOptions(
			controller.Options{
				MaxConcurrentReconciles: r.Max(),
				Log:                     mgr.GetLogger().WithName("controller").WithName(ControllerIPAM),
			}).
		Complete(r)
}
