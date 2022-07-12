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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

const ControllerQuota = "Quota"

// QuotaReconciler reconciles quota labels on node
type QuotaReconciler struct {
	client.Client

	concurrency.ControllerConcurrency
}

func (r *QuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	var network = &networkingv1.Network{}

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	if err = r.Get(ctx, req.NamespacedName, network); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch Network", client.IgnoreNotFound(err))
	}

	if networkingv1.GetNetworkType(network) != networkingv1.NetworkTypeUnderlay {
		log.V(1).Info("only underlay network need quota labels")
		return ctrl.Result{}, nil
	}

	var quotaLabels = map[string]string{
		constants.LabelIPv4AddressQuota:      constants.QuotaEmpty,
		constants.LabelIPv6AddressQuota:      constants.QuotaEmpty,
		constants.LabelDualStackAddressQuota: constants.QuotaEmpty,
	}
	if networkingv1.IsAvailable(network.Status.Statistics) {
		quotaLabels[constants.LabelIPv4AddressQuota] = constants.QuotaNonEmpty
	}
	if networkingv1.IsAvailable(network.Status.IPv6Statistics) {
		quotaLabels[constants.LabelIPv6AddressQuota] = constants.QuotaNonEmpty
	}
	if networkingv1.IsAvailable(network.Status.DualStackStatistics) {
		quotaLabels[constants.LabelDualStackAddressQuota] = constants.QuotaNonEmpty
	}

	var patchFuncs []func() error
	for _, nodeName := range network.Status.NodeList {
		nodeNameCopy := nodeName
		patchFuncs = append(patchFuncs, func() error {
			return r.patchNodeLabels(ctx, nodeNameCopy, quotaLabels)
		})
	}

	if err = errors.AggregateGoroutines(patchFuncs...); err != nil {
		return ctrl.Result{}, wrapError("unable to update quota labels", err)
	}

	log.V(1).Info(fmt.Sprintf("sync nodes %v quota labels to %v",
		network.Status.NodeList, quotaLabels))
	return ctrl.Result{}, nil
}

func (r *QuotaReconciler) patchNodeLabels(ctx context.Context, nodeName string, labels map[string]string) error {
	if len(labels) == 0 {
		return nil
	}

	marshaledLabels, err := json.Marshal(labels)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return client.IgnoreNotFound(r.Patch(ctx,
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			client.RawPatch(
				types.MergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":%s}}`, string(marshaledLabels))),
			),
		))
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *QuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerQuota).
		For(&networkingv1.Network{},
			builder.WithPredicates(
				&predicate.ResourceVersionChangedPredicate{},
				&utils.NetworkStatusChangePredicate{},
			)).
		WithOptions(
			controller.Options{
				MaxConcurrentReconciles: r.Max(),
			},
		).
		Complete(r)
}
