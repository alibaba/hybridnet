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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
)

const ControllerNode = "Node"

// NodeReconciler reconciles a Node object
type NodeReconciler struct {
	client.Client

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=nodes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Node object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	var node = &corev1.Node{}
	var err error
	if err = r.Get(ctx, req.NamespacedName, node); err != nil {
		log.Error(err, "unable to fetch Node")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var underlayAttached, overlayAttached, bgpAttached bool
	if underlayAttached, overlayAttached, err = utils.DetectNetworkAttachmentOfNode(r, node); err != nil {
		log.Error(err, "unable to detect network attachment")
		return ctrl.Result{}, err
	}

	if underlayAttached {
		if globalBGPNetwork, err := utils.FindGlobalBGPNetwork(r); err != nil {
			log.Error(err, "unable to detect global bgp network exist")
			return ctrl.Result{}, err
		} else if len(globalBGPNetwork) != 0 {
			// if global bgp network not exist, do not patch bgp attachment label to node
			var bgpNetworkName string
			if bgpNetworkName, err = utils.FindBGPUnderlayNetworkForNode(r, node.GetLabels()); err != nil {
				log.Error(err, "unable to find bgp underlay network for node %v", node.Name)
				return ctrl.Result{}, err
			}

			if len(bgpNetworkName) != 0 {
				bgpAttached = true
			}
		}
	}

	nodePatch := client.MergeFrom(node.DeepCopy())

	updateAttachmentLabel := func(node *corev1.Node, key string, attached bool) {
		if attached {
			if node.Labels == nil {
				node.Labels = map[string]string{}
			}
			node.Labels[key] = constants.Attached
			return
		}
		delete(node.Labels, key)
	}

	updateAttachmentLabel(node, constants.LabelUnderlayNetworkAttachment, underlayAttached)
	updateAttachmentLabel(node, constants.LabelOverlayNetworkAttachment, overlayAttached)
	updateAttachmentLabel(node, constants.LabelBGPNetworkAttachment, bgpAttached)

	if err = r.Patch(ctx, node, nodePatch); err != nil {
		log.Error(err, "unable to patch Node")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func nodeNamesToReconcileRequests(nodeNames []string) []reconcile.Request {
	ret := make([]reconcile.Request, len(nodeNames))
	for i := range nodeNames {
		ret[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: nodeNames[i],
			},
		}
	}
	return ret
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerNode).
		For(&corev1.Node{},
			builder.WithPredicates(
				&utils.IgnoreDeletePredicate{},
				&predicate.ResourceVersionChangedPredicate{},
				&predicate.LabelChangedPredicate{},
				&utils.NetworkOfNodeChangePredicate{Client: r},
			)).
		Watches(&source.Kind{Type: &networkingv1.Network{}},
			handler.EnqueueRequestsFromMapFunc(
				// enqueue all nodes here
				func(_ client.Object) []reconcile.Request {
					// TODO: handle error here
					nodeNames, _ := utils.ListNodesToNames(r)
					return nodeNamesToReconcileRequests(nodeNames)
				},
			),
			builder.WithPredicates(
				&predicate.GenerationChangedPredicate{},
				&utils.NetworkSpecChangePredicate{},
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
		}).
		Complete(r)
}
