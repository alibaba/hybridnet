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

	corev1 "k8s.io/api/core/v1"
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
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	globalutils "github.com/alibaba/hybridnet/pkg/utils"
)

const ControllerQuota = "Quota"

// QuotaReconciler reconciles quota labels on node
type QuotaReconciler struct {
	context.Context
	client.Client

	concurrency.ControllerConcurrency
}

func (r *QuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	var node = &corev1.Node{}

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	if err = r.Get(ctx, req.NamespacedName, node); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch Node", client.IgnoreNotFound(err))
	}

	// get related underlay network
	var networkList *networkingv1.NetworkList
	if networkList, err = utils.ListNetworks(ctx, r, client.MatchingFields{IndexerFieldNode: req.Name}); err != nil {
		return ctrl.Result{}, wrapError("unable to list underlay network by indexer", err)
	}

	nodePatch := client.MergeFrom(node.DeepCopy())

	// if no matched underlay network, clean the unnecessary quota labels
	if len(networkList.Items) == 0 {
		if node.Labels == nil {
			return ctrl.Result{}, nil
		}
		delete(node.Labels, constants.LabelIPv4AddressQuota)
		delete(node.Labels, constants.LabelIPv6AddressQuota)
		delete(node.Labels, constants.LabelDualStackAddressQuota)

		return ctrl.Result{}, wrapError("unable to clean node quota labels", r.Patch(ctx, node, nodePatch))
	}

	// TODO: use the first matched underlay network until multiple network selection supported
	network := &networkList.Items[0]

	valueFromAvailable := func(available bool) string {
		if available {
			return constants.QuotaNonEmpty
		}
		return constants.QuotaEmpty
	}

	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	node.Labels[constants.LabelIPv4AddressQuota] = valueFromAvailable(networkingv1.IsAvailable(network.Status.Statistics))
	node.Labels[constants.LabelIPv6AddressQuota] = valueFromAvailable(networkingv1.IsAvailable(network.Status.IPv6Statistics))
	node.Labels[constants.LabelDualStackAddressQuota] = valueFromAvailable(networkingv1.IsAvailable(network.Status.DualStackStatistics))

	if err = r.Patch(ctx, node, nodePatch); err != nil {
		return ctrl.Result{}, wrapError("unable to update quota labels", err)
	}

	log.V(1).Info(fmt.Sprintf("sync quota labels to %v", node.Labels))
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *QuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerQuota).
		For(&corev1.Node{},
			builder.WithPredicates(
				&utils.IgnoreDeletePredicate{},
				&predicate.ResourceVersionChangedPredicate{},
				&predicate.LabelChangedPredicate{},
				&utils.NetworkOfNodeChangePredicate{
					Context: r.Context,
					Client:  r.Client,
				},
			)).
		Watches(&source.Kind{Type: &networkingv1.Network{}},
			handler.EnqueueRequestsFromMapFunc(
				func(obj client.Object) []reconcile.Request {
					network, ok := obj.(*networkingv1.Network)
					if !ok {
						return nil
					}

					// enqueue network-related nodes
					return nodeNamesToReconcileRequests(globalutils.DeepCopyStringSlice(network.Status.NodeList))
				},
			),
			builder.WithPredicates(
				// only nodes of underlay network need quota labels
				utils.NetworkTypePredicate(networkingv1.NetworkTypeUnderlay),
				&predicate.ResourceVersionChangedPredicate{},
				&utils.NetworkStatusChangePredicate{},
			),
		).
		WithOptions(
			controller.Options{
				MaxConcurrentReconciles: r.Max(),
				RecoverPanic:            true,
			},
		).
		Complete(r)
}
