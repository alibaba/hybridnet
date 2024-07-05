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
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/metrics"
)

const (
	ControllerNetworkStatus = "NetworkStatus"
	IndexerFieldNetwork     = "network"
)

// NetworkStatusReconciler reconciles status of network objects
type NetworkStatusReconciler struct {
	context.Context
	client.Client

	IPAMManager IPAMManager
	Recorder    record.EventRecorder

	NetworkStatusUpdateChan <-chan event.GenericEvent

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.alibaba.com,resources=networks/finalizers,verbs=update

func (r *NetworkStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	var network = &networkingv1.Network{}

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
			if len(network.UID) > 0 {
				r.Recorder.Event(network, corev1.EventTypeWarning, "UpdateStatusFail", err.Error())
			}
		}
	}()

	if err = r.Get(ctx, req.NamespacedName, network); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch Network", client.IgnoreNotFound(err))
	}

	if network.DeletionTimestamp != nil {
		cleanMetrics(network.Name)
		return ctrl.Result{}, wrapError("unable to remove finalizer", r.removeFinalizer(ctx, network))

	}

	// make sure metrics will be un-registered before deletion
	if err = r.addFinalizer(ctx, network); err != nil {
		return ctrl.Result{}, wrapError("unable to add finalizer to network", err)
	}

	nodeSelector := network.Spec.NodeSelector
	switch networkingv1.GetNetworkType(network) {
	case networkingv1.NetworkTypeGlobalBGP:
		nodeSelector = map[string]string{
			constants.LabelBGPNetworkAttachment: constants.Attached,
		}
	}

	// update node list
	networkStatus := &networkingv1.NetworkStatus{}
	if networkStatus.NodeList, err = utils.ListActiveNodesToNames(ctx, r, client.MatchingLabels(nodeSelector)); err != nil {
		return ctrl.Result{}, wrapError("unable to update node list", err)
	}
	sort.Strings(networkStatus.NodeList)

	// update subnet list
	if networkStatus.SubnetList, err = utils.ListActiveSubnetsToNames(ctx,
		r,
		client.MatchingFields{
			IndexerFieldNetwork: network.GetName(),
		},
	); err != nil {
		return ctrl.Result{}, wrapError("unable to update subnet list", err)
	}
	sort.Strings(networkStatus.SubnetList)

	var networkUsage *ipamtypes.NetworkUsage
	if networkUsage, err = r.IPAMManager.GetNetworkUsage(network.GetName()); err != nil {
		return ctrl.Result{}, wrapError("unable to fetch network usage", err)
	}

	if ipv4Usage := networkUsage.GetByType(ipamtypes.IPv4); ipv4Usage != nil {
		networkStatus.LastAllocatedSubnet = ipv4Usage.LastAllocation
		networkStatus.Statistics = &networkingv1.Count{
			Total:     int32(ipv4Usage.Total),
			Used:      int32(ipv4Usage.Used),
			Available: int32(ipv4Usage.Available),
		}
	}
	if ipv6Usage := networkUsage.GetByType(ipamtypes.IPv6); ipv6Usage != nil {
		networkStatus.LastAllocatedIPv6Subnet = ipv6Usage.LastAllocation
		networkStatus.IPv6Statistics = &networkingv1.Count{
			Total:     int32(ipv6Usage.Total),
			Used:      int32(ipv6Usage.Used),
			Available: int32(ipv6Usage.Available),
		}
	}
	if dualStackUsage := networkUsage.GetByType(ipamtypes.DualStack); dualStackUsage != nil {
		networkStatus.DualStackStatistics = &networkingv1.Count{
			Available: int32(dualStackUsage.Available),
		}
	}

	// diff for no-op
	if reflect.DeepEqual(&network.Status, networkStatus) {
		log.V(1).Info("network status is up-to-date, skip updating")
		return ctrl.Result{}, nil
	}

	// update metrics
	updateUsageMetrics(network.Name, networkStatus)

	// patch network status
	networkPatch := client.MergeFrom(network.DeepCopy())
	network.Status = *networkStatus
	if err = retry.RetryOnConflict(retry.DefaultRetry,
		func() error {
			return r.Status().Patch(ctx, network, networkPatch)
		},
	); err != nil {
		return ctrl.Result{}, wrapError("unable to update network status", err)
	}

	log.V(1).Info(fmt.Sprintf("sync network status to %+v", networkStatus))
	return ctrl.Result{}, nil
}

func updateUsageMetrics(networkName string, networkStatus *networkingv1.NetworkStatus) {
	if networkStatus.Statistics != nil {
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPv4, metrics.IPTotalUsageType).
			Set(float64(networkStatus.Statistics.Total))
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPv4, metrics.IPUsedUsageType).
			Set(float64(networkStatus.Statistics.Used))
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPv4, metrics.IPAvailableUsageType).
			Set(float64(networkStatus.Statistics.Available))
	}

	if networkStatus.IPv6Statistics != nil {
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPv6, metrics.IPTotalUsageType).
			Set(float64(networkStatus.IPv6Statistics.Total))
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPv6, metrics.IPUsedUsageType).
			Set(float64(networkStatus.IPv6Statistics.Used))
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.IPv6, metrics.IPAvailableUsageType).
			Set(float64(networkStatus.IPv6Statistics.Available))
	}

	if networkStatus.DualStackStatistics != nil {
		metrics.IPUsageGauge.WithLabelValues(networkName, metrics.DualStack, metrics.IPAvailableUsageType).
			Set(float64(networkStatus.DualStackStatistics.Available))
	}
}

func cleanMetrics(networkName string) {
	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.IPv4, metrics.IPTotalUsageType)
	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.IPv4, metrics.IPUsedUsageType)
	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.IPv4, metrics.IPAvailableUsageType)

	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.IPv6, metrics.IPTotalUsageType)
	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.IPv6, metrics.IPUsedUsageType)
	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.IPv6, metrics.IPAvailableUsageType)

	_ = metrics.IPUsageGauge.DeleteLabelValues(networkName, metrics.DualStack, metrics.IPAvailableUsageType)
}

func (r *NetworkStatusReconciler) addFinalizer(ctx context.Context, network *networkingv1.Network) error {
	if controllerutil.ContainsFinalizer(network, constants.FinalizerMetricsRegistered) {
		return nil
	}

	patch := client.MergeFrom(network.DeepCopy())
	controllerutil.AddFinalizer(network, constants.FinalizerMetricsRegistered)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, network, patch)
	})
}

func (r *NetworkStatusReconciler) removeFinalizer(ctx context.Context, network *networkingv1.Network) error {
	if !controllerutil.ContainsFinalizer(network, constants.FinalizerMetricsRegistered) {
		return nil
	}

	patch := client.MergeFrom(network.DeepCopy())
	controllerutil.RemoveFinalizer(network, constants.FinalizerMetricsRegistered)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, network, patch)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkStatusReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerNetworkStatus).
		For(&networkingv1.Network{},
			builder.WithPredicates(
				&utils.IgnoreDeletePredicate{},
				&predicate.ResourceVersionChangedPredicate{},
				predicate.Or(
					&utils.NetworkSpecChangePredicate{},
					&utils.TerminatingPredicate{},
				),
			)).
		Watches(&source.Kind{Type: &networkingv1.Subnet{}},
			handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
				subnet, ok := object.(*networkingv1.Subnet)
				if !ok {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name: subnet.Spec.Network,
						},
					},
				}
			}),
			builder.WithPredicates(
				predicate.Or(
					&utils.TerminatingPredicate{},
					predicate.And(
						&predicate.GenerationChangedPredicate{},
						&utils.SubnetSpecChangePredicate{},
					),
				),
			)).
		Watches(&source.Kind{Type: &networkingv1.IPInstance{}},
			handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
				ipInstance, ok := object.(*networkingv1.IPInstance)
				if !ok {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name: ipInstance.Spec.Network,
						},
					},
				}
			}),
			builder.WithPredicates(
				&utils.IgnoreUpdatePredicate{},
			)).
		Watches(&source.Kind{Type: &corev1.Node{}},
			handler.EnqueueRequestsFromMapFunc(func(object client.Object) (ret []reconcile.Request) {
				node, ok := object.(*corev1.Node)
				if !ok {
					return nil
				}
				// ignore error
				underlayNetworkName, _ := utils.FindUnderlayNetworkForNode(r.Context, r, node.GetLabels())
				if len(underlayNetworkName) > 0 {
					ret = append(ret, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: underlayNetworkName,
						},
					})
				}

				// ignore error
				overlayNetworkName, _ := utils.FindOverlayNetwork(r.Context, r)
				if len(overlayNetworkName) > 0 {
					ret = append(ret, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: overlayNetworkName,
						},
					})
				}

				// ignore error
				globalBGPNetworkName, _ := utils.FindGlobalBGPNetwork(r.Context, r)
				if len(globalBGPNetworkName) > 0 {
					ret = append(ret, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: globalBGPNetworkName,
						},
					})
				}
				return
			}),
			builder.WithPredicates(
				&predicate.ResourceVersionChangedPredicate{},
				predicate.Or(
					&utils.TerminatingPredicate{},
					predicate.And(
						&predicate.LabelChangedPredicate{},
						&utils.NetworkOfNodeChangePredicate{
							Context: r.Context,
							Client:  r.Client,
						},
					),
				),
			)).
		Watches(&source.Channel{Source: r.NetworkStatusUpdateChan, DestBufferSize: 100},
			&handler.EnqueueRequestForObject{},
		).
		WithOptions(
			controller.Options{
				MaxConcurrentReconciles: r.Max(),
				RecoverPanic:            true,
			},
		).
		Complete(r)
}
