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

	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/client-go/tools/record"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"

	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ControllerIPInstanceStatus = "NetworkStatus"
)

// IPInstanceStatusReconciler reconciles status of IPInstance objects
type IPInstanceStatusReconciler struct {
	client.Client

	Recorder record.EventRecorder

	concurrency.ControllerConcurrency
}

func (r *IPInstanceStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	var ip = &networkingv1.IPInstance{}

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
			if len(ip.UID) > 0 {
				r.Recorder.Event(ip, corev1.EventTypeWarning, "UpdateStatusFail", err.Error())
			}
		}
	}()

	if err = r.Get(ctx, req.NamespacedName, ip); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch IPInstance", client.IgnoreNotFound(err))
	}

	if err = r.updateIPStatus(ip,
		networkingv1.GetNodeOfIPInstance(ip),
		networkingv1.GetPodOfIPInstance(ip),
		ip.Namespace,
		networkingv1.GetPhaseOfIPInstance(ip)); err != nil {
		return ctrl.Result{}, wrapError("unable to update IPInstance status", err)
	}

	return ctrl.Result{}, nil
}

func (r *IPInstanceStatusReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerIPInstanceStatus).
		For(&networkingv1.IPInstance{},
			builder.WithPredicates(
				&predicate.LabelChangedPredicate{},
				&utils.IgnoreDeletePredicate{},
			)).
		Complete(r)
}

func (r *IPInstanceStatusReconciler) updateIPStatus(ip *networkingv1.IPInstance, nodeName, podName, podNamespace string,
	phase networkingv1.IPPhase) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Status().Patch(context.TODO(),
			ip,
			client.RawPatch(
				types.MergePatchType,
				[]byte(fmt.Sprintf(
					`{"status":{"podName":%q,"podNamespace":%q,"nodeName":%q,"phase":%q}}`,
					podName,
					podNamespace,
					nodeName,
					phase,
				)),
			),
		)
	})
}
