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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
)

const ControllerIPInstance = "IPInstance"

// IPInstanceReconciler reconciles a IPInstance object
type IPInstanceReconciler struct {
	client.Client

	// TODO: construct
	PodIPCache  PodIPCache
	IPAMManager IPAMManager
	IPAMStore   IPAMStore

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups=networking.alibaba.com,resources=ipinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=ipinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=ipinstances/finalizers,verbs=update

func (r *IPInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	var ip networkingv1.IPInstance
	if err = r.Get(ctx, req.NamespacedName, &ip); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch IPInstance", client.IgnoreNotFound(err))
	}

	if !ip.DeletionTimestamp.IsZero() {
		podName := networkingv1.FetchBindingPodName(&ip)
		if len(podName) == 0 {
			return ctrl.Result{}, wrapError("cannot release an IPInstance without pod label", err)
		}

		r.PodIPCache.Release(ip.Name, ip.Namespace)

		if err = r.releaseIP(ctx, &ip); err != nil {
			return ctrl.Result{}, wrapError("unable to release IPInstance", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *IPInstanceReconciler) releaseIP(ctx context.Context, ipInstance *networkingv1.IPInstance) (err error) {
	if feature.DualStackEnabled() {
		if err = r.IPAMManager.DualStack().Release(utils.ToIPFamilyMode(networkingv1.IsIPv6IPInstance(ipInstance)),
			ipInstance.Spec.Network,
			[]string{
				ipInstance.Spec.Subnet,
			},
			[]string{
				utils.ToIPFormat(ipInstance.Name),
			},
		); err != nil {
			return err
		}
	} else {
		if err = r.IPAMManager.Release(ipInstance.Spec.Network, ipInstance.Spec.Subnet, utils.ToIPFormat(ipInstance.Name)); err != nil {
			return err
		}
	}
	if err = r.IPAMStore.IPUnBind(ctx, ipInstance.Namespace, ipInstance.Name); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerIPInstance).
		For(&networkingv1.IPInstance{}, builder.WithPredicates(
			&utils.IgnoreDeletePredicate{},
			&predicate.ResourceVersionChangedPredicate{},
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
		}).
		Complete(r)
}
