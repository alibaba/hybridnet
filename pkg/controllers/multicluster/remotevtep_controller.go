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
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

const ControllerRemoteVTEP = "RemoteVTEP"
const indexerFieldNode = "node"

//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remotevteps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remotevteps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remotevteps/finalizers,verbs=update

// RemoteVtepReconciler reconciles a Node object to RemoveVtep in parent cluster
type RemoteVtepReconciler struct {
	client.Client

	ClusterName         string
	ParentCluster       cluster.Cluster
	ParentClusterObject *multiclusterv1.RemoteCluster
}

func (r *RemoteVtepReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx).WithValues("Cluster", r.ClusterName)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	var node = &corev1.Node{}
	if err = r.Get(ctx, req.NamespacedName, node); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, wrapError("unable to clean VTEP for node", r.cleanVTEPForNode(ctx, req.Name))
		}
		return ctrl.Result{}, wrapError("unable to get node", err)
	}

	if !node.DeletionTimestamp.IsZero() {
		log.V(10).Info("ignore terminating node")
		return ctrl.Result{}, nil
	}

	var vtepIP, vtepMac, vtepVxlanIPList = node.Annotations[constants.AnnotationNodeVtepIP], node.Annotations[constants.AnnotationNodeVtepMac], node.Annotations[constants.AnnotationNodeLocalVxlanIPList]
	if len(vtepIP) == 0 || len(vtepMac) == 0 {
		log.V(10).Info("ignore node without vtep IP or MAC")
		return ctrl.Result{}, nil
	}

	var endpointIPList []string
	if endpointIPList, err = r.pickEndpointIPListForNode(ctx, req.Name); err != nil {
		return ctrl.Result{}, wrapError("unable to pick endpoint IP list for node", err)
	}

	var operationResult controllerutil.OperationResult
	var remoteVTEP = &multiclusterv1.RemoteVtep{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateVTEPName(r.ClusterName, req.Name),
		},
	}
	if operationResult, err = controllerutil.CreateOrPatch(ctx, r.ParentCluster.GetClient(), remoteVTEP, func() error {
		if !remoteVTEP.DeletionTimestamp.IsZero() {
			return fmt.Errorf("remote VTEP %s is terminating, can not be updated", remoteVTEP.Name)
		}

		if !metav1.IsControlledBy(remoteVTEP, r.ParentClusterObject) {
			if err = controllerutil.SetOwnerReference(r.ParentClusterObject, remoteVTEP, r.ParentCluster.GetScheme()); err != nil {
				return wrapError("unable to set owner reference", err)
			}
		}

		if remoteVTEP.Labels == nil {
			remoteVTEP.Labels = make(map[string]string)
		}
		remoteVTEP.Labels[constants.LabelCluster] = r.ClusterName
		remoteVTEP.Labels[constants.LabelNode] = node.Name

		if remoteVTEP.Annotations == nil {
			remoteVTEP.Annotations = make(map[string]string)
		}
		remoteVTEP.Annotations[constants.AnnotationNodeLocalVxlanIPList] = vtepVxlanIPList

		remoteVTEP.Spec.ClusterName = r.ClusterName
		remoteVTEP.Spec.NodeName = req.Name
		remoteVTEP.Spec.VTEPInfo = multiclusterv1.VTEPInfo{
			IP:  vtepIP,
			MAC: vtepMac,
		}
		remoteVTEP.Spec.EndpointIPList = endpointIPList
		return nil
	}); err != nil {
		return ctrl.Result{}, wrapError("unable to update VTEP", err)
	}

	if operationResult == controllerutil.OperationResultNone {
		log.V(10).Info("remote VTEP is up-to-date", "RemoteVTEP", remoteVTEP.Name)
		return ctrl.Result{}, nil
	}

	if err = r.ParentCluster.GetClient().Status().Patch(ctx, remoteVTEP, client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"status":{"lastModifyTime":%s}}`, metav1.Now())))); err != nil {
		// this error is not fatal, print it and go on
		log.Error(err, "unable to update VTEP status")
	}

	log.V(4).Info("update VTEP successfully", "RemoteVTEPSpec", remoteVTEP.Spec)
	return ctrl.Result{}, nil
}

func (r *RemoteVtepReconciler) cleanVTEPForNode(ctx context.Context, nodeName string) error {
	return client.IgnoreNotFound(r.ParentCluster.GetClient().Delete(ctx, &multiclusterv1.RemoteVtep{ObjectMeta: metav1.ObjectMeta{Name: generateVTEPName(r.ClusterName, nodeName)}}))
}

func (r *RemoteVtepReconciler) pickEndpointIPListForNode(ctx context.Context, nodeName string) ([]string, error) {
	ipInstanceList, err := utils.ListIPInstances(r, client.MatchingFields{indexerFieldNode: nodeName})
	if err != nil {
		return nil, err
	}

	var endpoints = make([]string, 0)
	for i := range ipInstanceList.Items {
		var ipInstance = &ipInstanceList.Items[i]
		// TODO: filter recognized subnet

		// only using IP will be valid endpoint
		if ipInstance == nil || ipInstance.Status.Phase != networkingv1.IPPhaseUsing {
			continue
		}
		endpointIP, _, _ := net.ParseCIDR(ipInstance.Spec.Address.IP)
		endpoints = append(endpoints, endpointIP.String())
	}

	// sort will make deep-equal stable
	sort.Strings(endpoints)
	return endpoints, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteVtepReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	gc := NewRemoteVTEPGarbageCollection(mgr.GetLogger().WithName("cron").WithName("RemoteVtepGC"), r)
	if err = mgr.Add(gc); err != nil {
		return err
	}

	// init node indexer for IP instances
	if err = mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.IPInstance{}, indexerFieldNode, func(obj client.Object) []string {
		nodeName := obj.GetLabels()[constants.LabelNode]
		if len(nodeName) > 0 {
			return []string{nodeName}
		}
		return nil
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerRemoteVTEP).
		For(&corev1.Node{},
			builder.WithPredicates(
				&predicate.ResourceVersionChangedPredicate{},
				&utils.SpecifiedAnnotationChangedPredicate{
					AnnotationKeys: []string{
						constants.AnnotationNodeVtepIP,
						constants.AnnotationNodeVtepMac,
						constants.AnnotationNodeLocalVxlanIPList,
					},
				},
			),
		).
		Watches(&source.Channel{Source: gc.EventChannel(), DestBufferSize: 100},
			&handler.EnqueueRequestForObject{},
		).
		// enqueue node if ip instances of node change
		Watches(&source.Kind{Type: &networkingv1.IPInstance{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				locatedNodeName := obj.GetLabels()[constants.LabelNode]
				if len(locatedNodeName) > 0 {
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name: locatedNodeName,
							},
						},
					}
				}
				return nil
			}),
			builder.WithPredicates(
				&predicate.ResourceVersionChangedPredicate{},
				// only IP instance with valid phase will be processed
				predicate.NewPredicateFuncs(func(obj client.Object) bool {
					ipInstance, ok := obj.(*networkingv1.IPInstance)
					if !ok {
						return false
					}
					return len(ipInstance.Status.Phase) > 0
				}),
				// if node or phase of IP instance change, node will be processed
				predicate.Or(
					&utils.SpecifiedLabelChangedPredicate{
						LabelKeys: []string{
							constants.LabelNode,
						},
					},
					&utils.IPInstancePhaseChangePredicate{},
				),
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}

func generateVTEPName(clusterName, nodeName string) string {
	return fmt.Sprintf("%s.%s", clusterName, nodeName)
}

func splitNodeNameFromRemoteVTEPName(remoteVTEPName string) string {
	return remoteVTEPName[strings.Index(remoteVTEPName, ".")+1:]
}
