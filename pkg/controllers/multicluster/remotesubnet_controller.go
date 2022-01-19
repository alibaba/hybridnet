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
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

// RemoteSubnetReconciler reconciles a RemoteSubnet object
type RemoteSubnetReconciler struct {
	client.Client

	ClusterName   string
	ParentCluster cluster.Cluster
}

//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remotesubnets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remotesubnets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.alibaba.com,resources=remotesubnets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RemoteSubnet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *RemoteSubnetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx).WithValues("Cluster", r.ClusterName)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
		}
	}()

	var subnet = &networkingv1.Subnet{}
	if err = r.Get(ctx, req.NamespacedName, subnet); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, wrapError("unable to clean remote subnet", r.cleanRemoteSubnet(ctx, req.Name))
		}
		return ctrl.Result{}, wrapError("unable to get subnet", err)
	}

	if !subnet.DeletionTimestamp.IsZero() {
		log.V(10).Info("ignore terminating subnet")
		return ctrl.Result{}, nil
	}

	var network = &networkingv1.Network{}
	if err = r.Get(ctx, types.NamespacedName{Name: subnet.Spec.Network}, network); err != nil {
		return ctrl.Result{}, wrapError("unable to get network", err)
	}

	var operationResult controllerutil.OperationResult
	var remoteSubnet = &multiclusterv1.RemoteSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateRemoteSubnetName(r.ClusterName, req.Name),
		},
	}
	if operationResult, err = controllerutil.CreateOrPatch(ctx, r.ParentCluster.GetClient(), remoteSubnet, func() error {
		if !remoteSubnet.DeletionTimestamp.IsZero() {
			return fmt.Errorf("remote subnet %s is terminating, can not be updated", remoteSubnet.Name)
		}

		// TODO: set owner reference of remote cluster

		if remoteSubnet.Labels == nil {
			remoteSubnet.Labels = make(map[string]string)
		}
		remoteSubnet.Labels[constants.LabelCluster] = r.ClusterName
		remoteSubnet.Labels[constants.LabelSubnet] = subnet.Name

		remoteSubnet.Spec.Type = network.Spec.Type
		remoteSubnet.Spec.Range = *subnet.Spec.Range.DeepCopy()
		remoteSubnet.Spec.ClusterName = r.ClusterName

		return nil
	}); err != nil {
		return ctrl.Result{}, wrapError("unable to update remote subnet", err)
	}

	if operationResult == controllerutil.OperationResultNone {
		log.V(10).Info("remote subnet is up-to-date", "RemoteSubnet", remoteSubnet.Name)
		return ctrl.Result{}, nil
	}

	if err = r.ParentCluster.GetClient().Status().Patch(ctx, remoteSubnet, client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"status":{"lastModifyTime":%s}}`, metav1.Now())))); err != nil {
		// this error is not fatal, print it and go on
		log.Error(err, "unable to update remote subnet status")
	}

	log.V(4).Info("update remote subnet successfully", "RemoteSubnetSpec", remoteSubnet.Spec)
	return ctrl.Result{}, nil
}

func (r *RemoteSubnetReconciler) cleanRemoteSubnet(ctx context.Context, subnetName string) error {
	return client.IgnoreNotFound(r.ParentCluster.GetClient().Delete(ctx, &multiclusterv1.RemoteSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateRemoteSubnetName(r.ClusterName, subnetName),
		},
	}))
}

func (r *RemoteSubnetReconciler) garbageCollection(ctx context.Context, log logr.Logger) (<-chan event.GenericEvent, error) {
	eventChannel := make(chan event.GenericEvent, 10)

	go func() {
		wait.UntilWithContext(ctx, func(c context.Context) {
			subnetList, err := utils.ListSubnets(r)
			if err != nil {
				log.Error(err, "unable to list subnets")
				return
			}
			var expectedRemoteSubnetSet = sets.NewString()
			for i := range subnetList.Items {
				expectedRemoteSubnetSet.Insert(generateRemoteSubnetName(r.ClusterName, subnetList.Items[i].Name))
			}

			// TODO: use indexer field instead of label selector
			var currentRemoteSubnetSet = sets.NewString()
			remoteSubnet, err := utils.ListRemoteSubnets(r.ParentCluster.GetClient(), client.MatchingLabels{constants.LabelCluster: r.ClusterName})
			if err != nil {
				log.Error(err, "unable to list remote subnets")
				return
			}
			for i := range remoteSubnet.Items {
				currentRemoteSubnetSet.Insert(remoteSubnet.Items[i].Name)
			}

			redundantRemoteSubnetSet := currentRemoteSubnetSet.Difference(expectedRemoteSubnetSet)
			for _, redundantRemoteSubnet := range redundantRemoteSubnetSet.UnsortedList() {
				// trigger this generic event for the missing (already deleted) objects
				eventChannel <- event.GenericEvent{
					Object: &networkingv1.Subnet{
						ObjectMeta: metav1.ObjectMeta{
							Name: splitSubnetNameFromRemoteSubnetName(redundantRemoteSubnet),
						},
					},
				}
			}
		}, time.Minute)

		close(eventChannel)
	}()

	return eventChannel, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteSubnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	garbageEvent, err := r.garbageCollection(context.TODO(), mgr.GetLogger().WithName("RemoteSubnetGC"))
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Subnet{},
			builder.WithPredicates(
				&predicate.GenerationChangedPredicate{},
			),
		).
		Watches(&source.Channel{Source: garbageEvent, DestBufferSize: 100},
			&handler.EnqueueRequestForObject{},
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			Log:                     mgr.GetLogger().WithName("RemoteSubnetController"),
		}).
		Complete(r)
}

func generateRemoteSubnetName(clusterName, subnetName string) string {
	return fmt.Sprintf("%s.%s", clusterName, subnetName)
}

func splitSubnetNameFromRemoteSubnetName(remoteSubnetName string) string {
	return remoteSubnetName[strings.Index(remoteSubnetName, ".")+1:]
}
