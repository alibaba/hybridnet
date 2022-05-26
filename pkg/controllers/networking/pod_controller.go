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
	"net"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/concurrency"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/metrics"
	globalutils "github.com/alibaba/hybridnet/pkg/utils"
	"github.com/alibaba/hybridnet/pkg/utils/transform"
)

const ControllerPod = "Pod"

const (
	ReasonIPAllocationSucceed = "IPAllocationSucceed"
	ReasonIPAllocationFail    = "IPAllocationFail"
	ReasonIPReleaseSucceed    = "IPReleaseSucceed"
	ReasonIPReserveSucceed    = "IPReserveSucceed"
)

const (
	IndexerFieldNode  = "node"
	OverlayNodeName   = "c3e6699d28e7"
	GlobalBGPNodeName = "d7afdca2c149"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	APIReader client.Reader
	client.Client

	Recorder record.EventRecorder

	PodIPCache  PodIPCache
	IPAMStore   IPAMStore
	IPAMManager IPAMManager

	concurrency.ControllerConcurrency
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=pods/finalizers,verbs=update

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrllog.FromContext(ctx)

	var (
		pod         = &corev1.Pod{}
		networkName string
	)

	defer func() {
		if err != nil {
			log.Error(err, "reconciliation fails")
			if len(pod.UID) > 0 {
				r.Recorder.Event(pod, corev1.EventTypeWarning, ReasonIPAllocationFail, err.Error())
			}
		}
	}()

	if err = r.Get(ctx, req.NamespacedName, pod); err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to fetch Pod: %v", err)
		}
		return ctrl.Result{}, nil
	}

	if pod.DeletionTimestamp != nil {
		if strategy.OwnByStatefulWorkload(pod) {
			if err = r.reserve(ctx, pod); err != nil {
				return ctrl.Result{}, wrapError("unable to reserve pod", err)
			}
			return ctrl.Result{}, wrapError("unable to remote finalizer", r.removeFinalizer(ctx, pod))
		}
		return ctrl.Result{}, nil
	}

	// Pre decouple ip instances for completed or evicted pods
	if utils.PodIsEvicted(pod) || utils.PodIsCompleted(pod) {
		return ctrl.Result{}, wrapError("unable to decouple pod", r.decouple(ctx, pod))
	}

	cacheExist, uid, ipInstanceList := r.PodIPCache.Get(pod.Name, pod.Namespace)
	// To avoid IP duplicate allocation
	if cacheExist && uid == pod.UID {
		ipFamily := ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])

		if (len(ipInstanceList) == 1 && (ipFamily == ipamtypes.IPv4Only || ipFamily == ipamtypes.IPv6Only)) ||
			(len(ipInstanceList) == 2 && ipFamily == ipamtypes.DualStack) {
			return ctrl.Result{}, nil
		}
	}

	networkName, err = r.selectNetwork(ctx, pod)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to select network: %v", err)
	}

	if strategy.OwnByStatefulWorkload(pod) {
		log.V(1).Info("strategic allocation for pod")
		return ctrl.Result{}, wrapError("unable to stateful allocate", r.statefulAllocate(ctx, pod, networkName))
	}

	return ctrl.Result{}, wrapError("unable to allocate", r.allocate(ctx, pod, networkName))
}

// decouple will unbind IP instance with Pod
func (r *PodReconciler) decouple(ctx context.Context, pod *corev1.Pod) (err error) {
	var decoupleFunc func(ctx context.Context, pod *corev1.Pod) (err error)
	if feature.DualStackEnabled() {
		decoupleFunc = r.IPAMStore.DualStack().DeCouple
	} else {
		decoupleFunc = r.IPAMStore.DeCouple
	}

	if err = decoupleFunc(ctx, pod); err != nil {
		return fmt.Errorf("unable to decouple ips for pod %s: %v", client.ObjectKeyFromObject(pod).String(), err)
	}

	r.Recorder.Event(pod, corev1.EventTypeNormal, ReasonIPReleaseSucceed, "pre decouple all IPs successfully")
	return nil
}

// reserve will reserve IP instances with Pod
func (r *PodReconciler) reserve(ctx context.Context, pod *corev1.Pod) (err error) {
	var reserveFunc func(ctx context.Context, pod *corev1.Pod) (err error)
	if feature.DualStackEnabled() {
		reserveFunc = r.IPAMStore.DualStack().IPReserve
	} else {
		reserveFunc = r.IPAMStore.IPReserve
	}

	if err = reserveFunc(ctx, pod); err != nil {
		return fmt.Errorf("unable to reserve ips for pod: %v", err)
	}

	r.Recorder.Event(pod, corev1.EventTypeNormal, ReasonIPReserveSucceed, "reserve all IPs successfully")
	return nil
}

// selectNetwork will pick the hit network by pod, taking the priority as below
// 1. explicitly specify network in pod annotations/labels
// 2. parse network type from pod and select a corresponding network binding on node
func (r *PodReconciler) selectNetwork(ctx context.Context, pod *corev1.Pod) (string, error) {
	var specifiedNetwork string
	if specifiedNetwork = globalutils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedNetwork], pod.Labels[constants.LabelSpecifiedNetwork]); len(specifiedNetwork) > 0 {
		return specifiedNetwork, nil
	}

	var networkType = types.ParseNetworkTypeFromString(globalutils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType]))
	switch networkType {
	case types.Underlay:
		// try to get underlay network by node indexer
		underlayNetworkName, err := r.getNetworkByNodeNameIndexer(ctx, pod.Spec.NodeName)
		if err != nil {
			return "", fmt.Errorf("unable to get underlay network by node name indexer: %v", err)
		}

		if len(underlayNetworkName) == 0 {
			return "", fmt.Errorf("unable to find underlay network for node %s", pod.Spec.NodeName)
		}

		if !r.matchNetworkTypeInManager(underlayNetworkName, types.Underlay) {
			return "", fmt.Errorf("network %s does not match type %q in manager", underlayNetworkName, types.Underlay)
		}
		return underlayNetworkName, nil
	case types.Overlay:
		// try to get overlay network by special node name
		overlayNetworkName, err := r.getNetworkByNodeNameIndexer(ctx, OverlayNodeName)
		if err != nil {
			return "", fmt.Errorf("unable to get overlay network by node name indexer: %v", err)
		}

		if len(overlayNetworkName) == 0 {
			return "", fmt.Errorf("unable to find overlay network")
		}

		if !r.matchNetworkTypeInManager(overlayNetworkName, types.Overlay) {
			return "", fmt.Errorf("network %s does not match type %q in manager", overlayNetworkName, types.Overlay)
		}
		return overlayNetworkName, nil
	case types.GlobalBGP:
		// try to get global bgp network by special node name
		globalBGPNetworkName, err := r.getNetworkByNodeNameIndexer(ctx, GlobalBGPNodeName)
		if err != nil {
			return "", fmt.Errorf("unable to get overlay network by node name indexer: %v", err)
		}

		if len(globalBGPNetworkName) == 0 {
			return "", fmt.Errorf("unable to find global bgp network")
		}

		if !r.matchNetworkTypeInManager(globalBGPNetworkName, types.GlobalBGP) {
			return "", fmt.Errorf("network %s does not match type %q in manager", globalBGPNetworkName, types.GlobalBGP)
		}
		return globalBGPNetworkName, nil
	default:
		return "", fmt.Errorf("unknown network type %s from pod", networkType)
	}
}

func (r *PodReconciler) getNetworkByNodeNameIndexer(ctx context.Context, nodeName string) (string, error) {
	var networkList *networkingv1.NetworkList
	var err error
	if networkList, err = utils.ListNetworks(ctx, r, client.MatchingFields{IndexerFieldNode: nodeName}); err != nil {
		return "", fmt.Errorf("unable to list network by indexer node name %v: %v", nodeName, err)
	}

	// only use the first one
	if len(networkList.Items) >= 1 {
		return networkList.Items[0].GetName(), nil
	}
	return "", nil
}

// matchNetworkTypeInManager will check the picked network from APIServer in manager on
// existence and type
// TODO: return error if non existing
func (r *PodReconciler) matchNetworkTypeInManager(networkName string, networkType types.NetworkType) bool {
	return (feature.DualStackEnabled() && r.IPAMManager.DualStack().MatchNetworkType(networkName, networkType)) ||
		(!feature.DualStackEnabled() && r.IPAMManager.MatchNetworkType(networkName, networkType))
}

func (r *PodReconciler) statefulAllocate(ctx context.Context, pod *corev1.Pod, networkName string) (err error) {
	var (
		preAssign     = len(pod.Annotations[constants.AnnotationIPPool]) > 0
		shouldObserve = true
		startTime     = time.Now()
		// reallocate means that ip should not be retained
		// 1. global retain and pod retain or unset, ip should be retained
		// 2. global retain and pod not retain, ip should be reallocated
		// 3. global not retain and pod not retain or unset, ip should be reallocated
		// 4. global not retain and pod retain, ip should be retained
		shouldReallocate = !globalutils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationIPRetain], strategy.DefaultIPRetain)
	)

	defer func() {
		if shouldObserve {
			metrics.IPAllocationPeriodSummary.
				WithLabelValues(metrics.IPStatefulAllocateType, strconv.FormatBool(err == nil)).
				Observe(float64(time.Since(startTime).Nanoseconds()))
		}
	}()

	if err = r.addFinalizer(ctx, pod); err != nil {
		return wrapError("unable to add finalizer for stateful pod", err)
	}

	if feature.DualStackEnabled() {
		var ipCandidates []string
		var ipFamilyMode = types.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])

		switch {
		case preAssign:
			ipPool := strings.Split(pod.Annotations[constants.AnnotationIPPool], ",")
			if idx := utils.GetIndexFromName(pod.Name); idx < len(ipPool) {
				ipCandidates = strings.Split(ipPool[idx], "/")
				for i := range ipCandidates {
					ipCandidates[i] = globalutils.NormalizedIP(ipCandidates[i])
				}
			} else {
				err = fmt.Errorf("no available ip in ip-pool %s", pod.Annotations[constants.AnnotationIPPool])
				return err
			}
		case shouldReallocate:
			var allocatedIPs []*networkingv1.IPInstance
			if allocatedIPs, err = utils.ListAllocatedIPInstancesOfPod(ctx, r, pod); err != nil {
				return err
			}

			// reallocate means that the allocated ones should be recycled firstly
			if len(allocatedIPs) > 0 {
				if err = r.release(ctx, pod, transform.TransferIPInstancesForIPAM(allocatedIPs)); err != nil {
					return wrapError("unable to release before reallocate", err)
				}
			}

			// reallocate
			return wrapError("unable to reallocate", r.allocate(ctx, pod, networkName))
		default:
			if ipCandidates, err = utils.ListIPsOfPod(ctx, r, pod); err != nil {
				return err
			}

			// when no valid ip found, it means that this is the first time of pod creation
			if len(ipCandidates) == 0 {
				// allocate has its own observation process, so just skip
				shouldObserve = false
				return wrapError("unable to allocate", r.allocate(ctx, pod, networkName))
			}
		}

		// forced assign for using reserved ips
		return wrapError("unable to multi-assign", r.multiAssign(ctx, pod, networkName, ipFamilyMode, ipCandidates, true))
	}

	var ipCandidate string

	switch {
	case preAssign:
		ipPool := strings.Split(pod.Annotations[constants.AnnotationIPPool], ",")
		if idx := utils.GetIndexFromName(pod.Name); idx < len(ipPool) {
			ipCandidate = globalutils.NormalizedIP(ipPool[idx])
		}
		if len(ipCandidate) == 0 {
			err = fmt.Errorf("no available ip in ip-pool %s", pod.Annotations[constants.AnnotationIPPool])
			return err
		}
	case shouldReallocate:
		var allocatedIPs []*networkingv1.IPInstance
		if allocatedIPs, err = utils.ListAllocatedIPInstancesOfPod(ctx, r, pod); err != nil {
			return err
		}

		// reallocate means that the allocated ones should be recycled firstly
		if len(allocatedIPs) > 0 {
			if err = r.release(ctx, pod, transform.TransferIPInstancesForIPAM(allocatedIPs)); err != nil {
				return wrapError("unable to release before reallocate", err)
			}
		}

		// reallocate
		return wrapError("unable to reallocate", r.allocate(ctx, pod, networkName))
	default:
		ipCandidate, err = utils.GetIPOfPod(ctx, r, pod)
		if err != nil {
			return err
		}
		// when no valid ip found, it means that this is the first time of pod creation
		if len(ipCandidate) == 0 {
			// allocate has its own observation process, so just skip
			shouldObserve = false
			return wrapError("unable to allocate", r.allocate(ctx, pod, networkName))
		}
	}

	// forced assign for using reserved ip
	return wrapError("unable to assign", r.assign(ctx, pod, networkName, ipCandidate, true))
}

// release will release IP instances of pod
func (r *PodReconciler) release(ctx context.Context, pod *corev1.Pod, allocatedIPs []*types.IP) (err error) {
	var recycleFunc func(ctx context.Context, namespace string, ip *types.IP) (err error)
	if feature.DualStackEnabled() {
		recycleFunc = r.IPAMStore.DualStack().IPRecycle
	} else {
		recycleFunc = r.IPAMStore.IPRecycle
	}

	for _, ip := range allocatedIPs {
		if err = recycleFunc(ctx, pod.Namespace, ip); err != nil {
			return fmt.Errorf("unable to recycle ip %v: %v", ip, err)
		}
	}

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPReleaseSucceed, "release IPs %v successfully", squashIPSliceToIPs(allocatedIPs))
	return nil
}

// allocate will allocate new IPs for pod
func (r *PodReconciler) allocate(ctx context.Context, pod *corev1.Pod, networkName string) (err error) {
	var startTime = time.Now()
	defer func() {
		metrics.IPAllocationPeriodSummary.
			WithLabelValues(metrics.IPNormalAllocateType, strconv.FormatBool(err == nil)).
			Observe(float64(time.Since(startTime).Nanoseconds()))
	}()

	if feature.DualStackEnabled() {
		var (
			subnetNames  []string
			ips          []*types.IP
			ipFamilyMode = types.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])
		)
		if subnetNameStr := globalutils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet], pod.Labels[constants.LabelSpecifiedSubnet]); len(subnetNameStr) > 0 {
			subnetNames = strings.Split(subnetNameStr, "/")
		}
		if ips, err = r.IPAMManager.DualStack().Allocate(ipFamilyMode, networkName, subnetNames, pod.Name, pod.Namespace); err != nil {
			return fmt.Errorf("unable to allocate %s ip: %v", ipFamilyMode, err)
		}
		defer func() {
			if err != nil {
				_ = r.IPAMManager.DualStack().Release(ipFamilyMode, networkName, squashIPSliceToSubnets(ips), squashIPSliceToIPs(ips))
			}
		}()

		if err = r.IPAMStore.DualStack().Couple(ctx, pod, ips); err != nil {
			return fmt.Errorf("unable to couple IPs with pod: %v", err)
		}

		var ipInstanceNames []string
		for _, ip := range ips {
			ipInstanceNames = append(ipInstanceNames, utils.ToDNSFormat(ip.Address.IP))
		}

		// Always keep updating pod ip cache the final step.
		r.PodIPCache.Record(pod.UID, pod.Name, pod.Namespace, ipInstanceNames)

		r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPAllocationSucceed, "allocate IPs %v successfully", squashIPSliceToIPs(ips))
		return nil
	}

	var (
		subnetName = globalutils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet], pod.Labels[constants.LabelSpecifiedSubnet])
		ip         *types.IP
	)
	if ip, err = r.IPAMManager.Allocate(networkName, subnetName, pod.Name, pod.Namespace); err != nil {
		return fmt.Errorf("unable to allocate ip: %v", err)
	}
	defer func() {
		if err != nil {
			_ = r.IPAMManager.Release(ip.Network, ip.Subnet, ip.Address.IP.String())
		}
	}()

	if err = r.IPAMStore.Couple(ctx, pod, ip); err != nil {
		return fmt.Errorf("unable to couple ip with pod: %v", err)
	}

	ipInstanceNames := []string{utils.ToDNSFormat(ip.Address.IP)}

	// Always keep updating pod ip cache the final step.
	r.PodIPCache.Record(pod.UID, pod.Name, pod.Namespace, ipInstanceNames)

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPAllocationSucceed, "allocate IP %s successfully", ip.String())
	return nil
}

// assign will reassign allocated IP to Pod
func (r *PodReconciler) assign(ctx context.Context, pod *corev1.Pod, networkName string, ipCandidate string, forced bool) (err error) {
	ip, err := r.IPAMManager.Assign(networkName, "", pod.Name, pod.Namespace, ipCandidate, forced)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = r.IPAMManager.Release(ip.Network, ip.Subnet, ip.Address.IP.String())
		}
	}()

	if err = r.IPAMStore.ReCouple(ctx, pod, ip); err != nil {
		return fmt.Errorf("unable to force-couple ip with pod: %v", err)
	}

	// Always keep updating pod ip cache the final step.
	r.PodIPCache.Record(pod.UID, pod.Name, pod.Namespace, []string{utils.ToDNSFormat(net.ParseIP(ipCandidate))})

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPAllocationSucceed, "assign IP %s successfully", ip.String())
	return nil
}

// multiAssign will reassign allcated IPs to Pod, usually used on dual stack mode
func (r *PodReconciler) multiAssign(ctx context.Context, pod *corev1.Pod, networkName string, ipFamily types.IPFamilyMode, ipCandidates []string, forced bool) (err error) {
	var IPs []*types.IP
	if IPs, err = r.IPAMManager.DualStack().Assign(ipFamily, networkName, nil, ipCandidates, pod.Name, pod.Namespace, forced); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = r.IPAMManager.DualStack().Release(ipFamily, networkName, squashIPSliceToSubnets(IPs), squashIPSliceToIPs(IPs))
		}
	}()

	if err = r.IPAMStore.DualStack().ReCouple(ctx, pod, IPs); err != nil {
		return fmt.Errorf("fail to force-couple ips %+v with pod: %v", IPs, err)
	}

	// Always keep updating pod ip cache the final step.
	var ipInstanceNames []string
	for _, candidate := range ipCandidates {
		ipInstanceNames = append(ipInstanceNames, utils.ToDNSFormat(net.ParseIP(candidate)))
	}
	r.PodIPCache.Record(pod.UID, pod.Name, pod.Namespace, ipInstanceNames)

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPAllocationSucceed, "assign IPs %v successfully", squashIPSliceToIPs(IPs))
	return nil
}

func (r *PodReconciler) addFinalizer(ctx context.Context, pod *corev1.Pod) error {
	if controllerutil.ContainsFinalizer(pod, constants.FinalizerIPAllocated) {
		return nil
	}

	patch := client.StrategicMergeFrom(pod.DeepCopy())
	controllerutil.AddFinalizer(pod, constants.FinalizerIPAllocated)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, pod, patch)
	})
}

func (r *PodReconciler) removeFinalizer(ctx context.Context, pod *corev1.Pod) error {
	if !controllerutil.ContainsFinalizer(pod, constants.FinalizerIPAllocated) {
		return nil
	}

	patch := client.StrategicMergeFrom(pod.DeepCopy())
	controllerutil.RemoveFinalizer(pod, constants.FinalizerIPAllocated)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Patch(ctx, pod, patch)
	})
}

func squashIPSliceToIPs(ips []*types.IP) (ret []string) {
	for _, ip := range ips {
		ret = append(ret, ip.Address.IP.String())
	}
	return
}

func squashIPSliceToSubnets(ips []*types.IP) (ret []string) {
	for _, ip := range ips {
		ret = append(ret, ip.Subnet)
	}
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerPod).
		For(&corev1.Pod{},
			builder.WithPredicates(
				&utils.IgnoreDeletePredicate{},
				&predicate.ResourceVersionChangedPredicate{},
				predicate.NewPredicateFuncs(func(obj client.Object) bool {
					pod, ok := obj.(*corev1.Pod)
					if !ok {
						return false
					}
					// ignore host networking pod
					if pod.Spec.HostNetwork {
						return false
					}

					if pod.DeletionTimestamp.IsZero() {
						// only pod after scheduling should be processed
						return len(pod.Spec.NodeName) > 0
					}

					// terminating pods owned by stateful workloads should be processed for IP reservation
					return strategy.OwnByStatefulWorkload(pod)
				}),
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
		}).
		Complete(r)
}
