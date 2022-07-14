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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
)

const ControllerIPAM = "IPAM"

// IPAMReconciler reconciles IPAM Manager
type IPAMReconciler struct {
	client.Client

	IPAMManager IPAMManager

	NetworkStatusUpdateChan chan<- event.GenericEvent
	SubnetStatusUpdateChan  chan<- event.GenericEvent

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks/finalizers,verbs=update

func (r *IPAMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	if err := r.IPAMManager.Refresh(ipamtypes.RefreshNetworks{req.Name}); err != nil {
		log.Error(err, "unable to refresh IPAM Manager", "network", req.Name)
		return ctrl.Result{}, err
	}

	// update statuses after refreshing network asynchronously
	go r.updateStatus(ctx, req.Name)

	return ctrl.Result{}, nil
}

// updateStatus will trigger some status reconciliations related to a network
func (r *IPAMReconciler) updateStatus(ctx context.Context, networkName string) {
	r.updateNetworkStatus(networkName)

	subnetNames, err := utils.ListActiveSubnetsToNames(ctx, r, client.MatchingFields{
		IndexerFieldNetwork: networkName,
	})
	if err != nil {
		ctrllog.FromContext(ctx).Error(err, "unable to list subnets")
		return
	}

	for i := range subnetNames {
		r.updateSubnetStatus(subnetNames[i])
	}
}

// updateNetworkStatus will trigger a reconciliation of network status controller
func (r *IPAMReconciler) updateNetworkStatus(networkName string) {
	r.NetworkStatusUpdateChan <- event.GenericEvent{
		Object: &networkingv1.Network{
			ObjectMeta: metav1.ObjectMeta{
				Name: networkName,
			},
		},
	}
}

// updateSubnetStatus will trigger a reconciliation of subnet status controller
func (r *IPAMReconciler) updateSubnetStatus(subnetName string) {
	r.SubnetStatusUpdateChan <- event.GenericEvent{
		Object: &networkingv1.Subnet{
			ObjectMeta: metav1.ObjectMeta{
				Name: subnetName,
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerIPAM).
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
			}).
		Complete(r)
}
