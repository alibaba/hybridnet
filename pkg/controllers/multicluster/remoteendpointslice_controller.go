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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
)

const (
	ControllerRemoteEndpointSlice     = "RemoteEndpointSlice"
	remoteEndpointSliceControllerName = "remote-endpointslice.hybridnet"
)

// RemoteEndpointSliceReconciler reconciles EndpointSlice objects of parent cluster
type RemoteEndpointSliceReconciler struct {
	context.Context
	client.Client

	ClusterName         string
	ParentCluster       cluster.Cluster
	ParentClusterObject *multiclusterv1.RemoteCluster
}

func (r *RemoteEndpointSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx).WithValues("Cluster", r.ClusterName)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	var remoteService = &corev1.Service{}
	if err = r.Get(ctx, req.NamespacedName, remoteService); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, wrapError("unable to get remote service", err)
	}

	if apierrors.IsNotFound(err) || !isGlobalService(remoteService) {
		return ctrl.Result{}, wrapError("unable to clean remote endpoint slices",
			r.cleanRemoteEndpointSlices(ctx, req.Name, req.Namespace))
	}

	return ctrl.Result{}, wrapError("unable to sync remote endpoint slices",
		r.syncRemoteEndpointSlices(ctx, remoteService.Name, remoteService.Namespace, log))
}

func (r *RemoteEndpointSliceReconciler) cleanRemoteEndpointSlices(ctx context.Context, service, namespace string) error {
	if err := r.ParentCluster.GetClient().DeleteAllOf(ctx, &multiclusterv1.RemoteEndpointSlice{},
		client.MatchingLabels(generateRemoteEndpointSliceLabels(service, r.ClusterName))); err != nil {
		return fmt.Errorf("unable to get remote endpoint slices of remote cluster %v for service %v/%v: %v",
			r.ClusterName, service, namespace, err)
	}
	return nil
}

func (r *RemoteEndpointSliceReconciler) syncRemoteEndpointSlices(ctx context.Context,
	service, namespace string, log logr.Logger) error {

	epsSel := labels.NewSelector()
	r1, _ := labels.NewRequirement(discoveryv1beta1.LabelServiceName, selection.Equals, []string{service})
	// avoid circular reference, exclude the EndpointSlices created by hybridnet
	r2, _ := labels.NewRequirement(discoveryv1beta1.LabelManagedBy, selection.NotEquals, []string{remoteEndpointSliceControllerName})
	epsSel = epsSel.Add(*r1, *r2)

	targetEpsList := &discoveryv1beta1.EndpointSliceList{}
	if err := r.List(ctx, targetEpsList, client.MatchingLabelsSelector{
		Selector: epsSel,
	}, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("unable to get endpoint slices of remote cluster %v for service %v/%v: %v",
			r.ClusterName, service, namespace, err)
	}

	existRepsList := &multiclusterv1.RemoteEndpointSliceList{}
	if err := r.ParentCluster.GetClient().List(ctx, existRepsList, client.MatchingLabels(generateRemoteEndpointSliceLabels(
		service, r.ClusterName))); err != nil {
		return fmt.Errorf("unable to get remote endpoint slices of remote cluster %v for service %v/%v: %v",
			r.ClusterName, service, namespace, err)
	}

	// TODO: Do we need a name prefix here for RemoteEndpointSlices?
	targetEpsMap := map[string]string{}
	for _, eps := range targetEpsList.Items {
		targetEpsMap[eps.Name] = eps.Name
	}

	for _, reps := range existRepsList.Items {
		if _, exist := targetEpsMap[reps.Name]; !exist {
			if err := client.IgnoreNotFound(r.ParentCluster.GetClient().Delete(ctx, &reps)); err != nil {
				return fmt.Errorf("unable to delete remote endpoint slice %v/%v: %v", reps.Name, reps.Namespace, err)
			}
		}
	}

	for index := range targetEpsList.Items {
		eps := targetEpsList.Items[index]

		var remoteEndpointSlice = &multiclusterv1.RemoteEndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: eps.Name,
			},
		}

		operationResult, err := controllerutil.CreateOrPatch(ctx, r.ParentCluster.GetClient(), remoteEndpointSlice, func() error {
			if !remoteEndpointSlice.DeletionTimestamp.IsZero() {
				return fmt.Errorf("endpoint slice %s is terminating, can not be updated", remoteEndpointSlice.Name)
			}

			// deep copy the labels and annotations of the endpoint slice
			for k, v := range eps.Labels {
				if remoteEndpointSlice.Labels == nil {
					remoteEndpointSlice.Labels = map[string]string{}
				}
				remoteEndpointSlice.Labels[k] = v
			}

			for k, v := range eps.Annotations {
				if remoteEndpointSlice.Annotations == nil {
					remoteEndpointSlice.Annotations = map[string]string{}
				}
				remoteEndpointSlice.Annotations[k] = v
			}

			remoteEndpointSlice.Labels[discoveryv1beta1.LabelManagedBy] = remoteEndpointSliceControllerName
			remoteEndpointSlice.Labels[constants.LabelRemoteCluster] = r.ClusterName

			remoteEndpointSlice.Spec.RemoteService.Cluster = r.ClusterName
			remoteEndpointSlice.Spec.RemoteService.Namespace = eps.Namespace
			remoteEndpointSlice.Spec.RemoteService.Name = service

			remoteEndpointSlice.Spec.AddressType = eps.AddressType
			remoteEndpointSlice.Spec.Endpoints = eps.Endpoints
			remoteEndpointSlice.Spec.Ports = eps.Ports

			if !metav1.IsControlledBy(remoteEndpointSlice, r.ParentClusterObject) {
				if err := controllerutil.SetOwnerReference(r.ParentClusterObject, remoteEndpointSlice,
					r.ParentCluster.GetScheme()); err != nil {
					return wrapError("unable to set owner reference", err)
				}
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("unable to update remote endpoint slice %v: %v", eps.Name, err)
		}

		if operationResult == controllerutil.OperationResultNone {
			log.V(1).Info("endpoint slice is up-to-date", "EndpoinSlice",
				eps.Name, "Namespace", eps.Namespace)
		}

		remoteEndpointSlicePatch := client.MergeFrom(remoteEndpointSlice.DeepCopy())
		remoteEndpointSlice.Status.LastModifyTime = metav1.Now()
		if err = r.ParentCluster.GetClient().Status().Patch(ctx, remoteEndpointSlice, remoteEndpointSlicePatch); err != nil {
			// this error is not fatal, print it and go on
			log.Error(err, "unable to update remote endpoint slice status")
		}
	}

	return nil
}

func (r *RemoteEndpointSliceReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerRemoteEndpointSlice).
		For(&corev1.Service{},
			builder.WithPredicates(
				globalServicePredicate(),
				predicate.Or(
					&predicate.GenerationChangedPredicate{},
					&predicate.AnnotationChangedPredicate{},
				),
			),
		).
		Watches(&source.Kind{Type: &discoveryv1beta1.EndpointSlice{}}, handler.EnqueueRequestsFromMapFunc(
			func(object client.Object) []reconcile.Request {
				eps, ok := object.(*discoveryv1beta1.EndpointSlice)
				if !ok {
					return nil
				}

				// ignore the "remote" EndpointSlice created by hybridnet
				if _, exist := eps.Labels[constants.LabelRemoteCluster]; exist {
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
		// TODO: Add periodic garbage collection?
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RecoverPanic:            true,
		}).
		Complete(r)
}

func generateRemoteEndpointSliceLabels(service, cluster string) map[string]string {
	return map[string]string{
		discoveryv1beta1.LabelServiceName: service,
		discoveryv1beta1.LabelManagedBy:   remoteEndpointSliceControllerName,
		constants.LabelRemoteCluster:      cluster,
	}
}
