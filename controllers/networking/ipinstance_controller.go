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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
	"github.com/alibaba/hybridnet/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
)

// IPInstanceReconciler reconciles a IPInstance object
type IPInstanceReconciler struct {
	client.Client

	// TODO: construct
	IPAMManager IPAMManager
	IPAMStore   IPAMStore
}

//+kubebuilder:rbac:groups=networking.alibaba.com,resources=ipinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=ipinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=ipinstances/finalizers,verbs=update

func (r *IPInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	var err error
	var ip networkingv1.IPInstance
	if err = r.Get(ctx, req.NamespacedName, &ip); err != nil {
		log.Error(err, "unable to fetch IPInstance")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ip.DeletionTimestamp.IsZero() {
		if err = r.releaseIP(&ip); err != nil {
			log.Error(err, "unable to release IPInstance")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *IPInstanceReconciler) releaseIP(ipInstance *networkingv1.IPInstance) (err error) {
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
		if err = r.IPAMStore.DualStack().IPUnBind(ipInstance.Namespace, ipInstance.Name); err != nil {
			return err
		}
	} else {
		if err = r.IPAMManager.Release(ipInstance.Spec.Network, ipInstance.Spec.Subnet, utils.ToIPFormat(ipInstance.Name)); err != nil {
			return err
		}
		if err = r.IPAMStore.IPUnBind(ipInstance.Namespace, ipInstance.Name); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.IPInstance{}).
		WithEventFilter(utils.IgnoreDeletePredicate{}).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			Log:                     mgr.GetLogger().WithName("IPInstanceController"),
		}).
		Complete(r)
}
