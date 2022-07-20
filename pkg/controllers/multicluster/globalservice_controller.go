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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"

	"github.com/alibaba/hybridnet/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
)

const ControllerGlobalService = "GlobalService"

// GlobalServiceReconciler reconciles a global Service object
type GlobalServiceReconciler struct {
	context.Context
	client.Client

	concurrency.ControllerConcurrency
}

func (r *GlobalServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	var service = &corev1.Service{}
	if err = r.Get(ctx, req.NamespacedName, service); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, wrapError("unable to get service", err)
	} else if apierrors.IsNotFound(err) || !service.DeletionTimestamp.IsZero() {
		// endpoints will be cleaned by owner reference
		return ctrl.Result{}, nil
	}

	if !isGlobalService(service) {
		return ctrl.Result{}, wrapError("unable to unbind remote endpoint slices",
			r.unbindRemoteEndpointSlices(ctx, service))
	}

	return ctrl.Result{}, wrapError("unable to bind remote endpoint slices",
		r.bindRemoteEndpointSlices(ctx, service))
}

func (r *GlobalServiceReconciler) unbindRemoteEndpointSlices(ctx context.Context, svc *corev1.Service) error {
	// delete all the endpoint slices created by hybridnet
	if err := r.DeleteAllOf(ctx, &discoveryv1beta1.EndpointSlice{}, client.MatchingLabels{
		discoveryv1beta1.LabelServiceName: svc.Name,
		discoveryv1beta1.LabelManagedBy:   remoteEndpointSliceControllerName,
	}, client.InNamespace(svc.Namespace)); err != nil {
		return fmt.Errorf("unable to list remote endpoint slices for service %v/%v: %v",
			svc.Name, svc.Namespace, err)
	}
	return nil
}

func (r *GlobalServiceReconciler) bindRemoteEndpointSlices(ctx context.Context, svc *corev1.Service) error {
	// sync endpoint slices according to remote endpoint slices
	epsList := &discoveryv1beta1.EndpointSliceList{}
	if err := r.List(ctx, epsList, client.MatchingLabels{
		discoveryv1beta1.LabelServiceName: svc.Name,
		discoveryv1beta1.LabelManagedBy:   remoteEndpointSliceControllerName,
	}, client.InNamespace(svc.Namespace)); err != nil {
		return fmt.Errorf("unable to list remote endpoint slices for service %v/%v: %v",
			svc.Name, svc.Namespace, err)
	}

	repsList := &multiclusterv1.RemoteEndpointSliceList{}
	if err := r.List(ctx, repsList, client.MatchingLabels{
		discoveryv1beta1.LabelServiceName: svc.Name,
	}); err != nil {
		return fmt.Errorf("unable to list remote endpoint slices for service %v/%v: %v",
			svc.Name, svc.Namespace, err)
	}

	repsMap := map[string]string{}
	for _, reps := range repsList.Items {
		if reps.DeletionTimestamp.IsZero() {
			repsMap[reps.Name] = reps.Name
		}
	}

	for _, eps := range epsList.Items {
		if _, exist := repsMap[eps.Name]; !exist {
			if err := client.IgnoreNotFound(r.Delete(ctx, &eps)); err != nil {
				return fmt.Errorf("unable to delete endpoint slice %v/%v: %v", eps.Name, eps.Namespace, err)
			}
		}
	}

	for index := range repsList.Items {
		reps := &repsList.Items[index]

		if reps.Spec.RemoteService.Namespace != svc.Namespace {
			continue
		}

		var endpointSlice = &discoveryv1beta1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      reps.Name,
				Namespace: reps.Spec.RemoteService.Namespace,
			},
		}

		_, err := controllerutil.CreateOrPatch(ctx, r, endpointSlice, func() error {
			if !endpointSlice.DeletionTimestamp.IsZero() {
				return fmt.Errorf("endpoint slice %s is terminating, can not be updated", endpointSlice.Name)
			}

			endpointSlice.Labels = reps.Labels
			endpointSlice.Annotations = reps.Annotations
			endpointSlice.Endpoints = reps.Spec.Endpoints
			endpointSlice.AddressType = reps.Spec.AddressType
			endpointSlice.Ports = reps.Spec.Ports

			if !metav1.IsControlledBy(endpointSlice, svc) {
				// this is the same as EndpointSlices create by kube-controller-manager
				if err := controllerutil.SetControllerReference(svc, endpointSlice, r.Scheme()); err != nil {
					return wrapError("unable to set owner reference", err)
				}
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("unable to update endpoint slice %v: %v", endpointSlice.Name, err)
		}
	}

	return nil
}

func (r *GlobalServiceReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerGlobalService).
		For(&corev1.Service{},
			builder.WithPredicates(
				globalServicePredicate(),
				predicate.Or(
					&predicate.GenerationChangedPredicate{},
					&predicate.AnnotationChangedPredicate{},
				),
			),
		).
		Watches(&source.Kind{Type: &multiclusterv1.RemoteEndpointSlice{}}, handler.EnqueueRequestsFromMapFunc(
			func(object client.Object) []reconcile.Request {
				reps, ok := object.(*multiclusterv1.RemoteEndpointSlice)
				if !ok {
					return nil
				}

				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      reps.Spec.RemoteService.Name,
							Namespace: reps.Spec.RemoteService.Namespace,
						},
					},
				}
			})).
		Watches(&source.Kind{Type: &discoveryv1beta1.EndpointSlice{}}, handler.EnqueueRequestsFromMapFunc(
			func(object client.Object) []reconcile.Request {
				eps, ok := object.(*discoveryv1beta1.EndpointSlice)
				if !ok {
					return nil
				}

				// only for the "remote" EndpointSlice created by hybridnet
				if _, exist := eps.Labels[constants.LabelRemoteCluster]; !exist {
					return nil
				}

				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      eps.GetLabels()[discoveryv1beta1.LabelServiceName],
							Namespace: eps.Namespace,
						},
					},
				}
			}),
			builder.WithPredicates(
				&predicate.ResourceVersionChangedPredicate{},
			)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
		}).
		Complete(r)
}

func globalServicePredicate() predicate.Funcs {
	globalServiceFilter := func(obj client.Object) bool {
		svc, ok := obj.(*corev1.Service)
		if !ok {
			return false
		}

		return isGlobalService(svc)
	}

	gsPredicate := predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return globalServiceFilter(createEvent.Object)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return globalServiceFilter(updateEvent.ObjectOld) || globalServiceFilter(updateEvent.ObjectNew)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return globalServiceFilter(deleteEvent.Object)
		},
	}

	return gsPredicate
}

func isGlobalService(service *corev1.Service) bool {
	return utils.ParseBoolOrDefault(service.GetAnnotations()[constants.AnnotationGlobalService], false)
}
