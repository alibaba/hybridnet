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
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"
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
	macutils "github.com/alibaba/hybridnet/pkg/utils/mac"
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
	IndexerFieldMAC   = "mac"
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
	PodSelector utils.PodSelector
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

	// We need to reserve ip for terminating, evicted and completed ip-retained pods.
	// For evicted and completed ip-retained pods, will be not reconciled while getting terminating, because
	// finalizer is removed.
	if pod.DeletionTimestamp != nil || utils.PodIsEvicted(pod) || utils.PodIsCompleted(pod) {
		var ownedObj client.Object = pod

		// For terminating pods with no controller owner reference, try to get
		// owner reference from ip instance.
		if metav1.GetControllerOf(pod) == nil {
			var ipInstanceList = &networkingv1.IPInstanceList{}
			if err = r.List(ctx, ipInstanceList,
				client.MatchingLabels{constants.LabelPod: transform.TransferPodNameForLabelValue(pod.Name)},
				client.InNamespace(pod.Namespace),
			); err != nil {
				return ctrl.Result{}, wrapError("failed to list ip instance for pod", err)
			}
			for i := range ipInstanceList.Items {
				ipInstance := &ipInstanceList.Items[i]
				if !ipInstance.DeletionTimestamp.IsZero() {
					continue
				}
				if metav1.GetControllerOf(ipInstance) != nil {
					ownedObj = ipInstance.DeepCopy()
					break
				}
			}

			// If we still cannot find owner ref on all related ip instances, just remove pod finalizer.
			if metav1.GetControllerOf(ownedObj) == nil {
				return ctrl.Result{}, wrapError("unable to remove finalizer", r.removeFinalizer(ctx, pod))
			}
		}

		if strategy.OwnByStatefulWorkload(ownedObj) {
			// Before pod terminated, should not reserve ip instance because of pre-stop
			if !utils.PodIsNotRunning(pod) {
				return ctrl.Result{}, nil
			}

			if err = r.reserve(ctx, pod); err != nil {
				return ctrl.Result{}, wrapError("unable to reserve pod", err)
			}
			return ctrl.Result{}, wrapError("unable to remove finalizer", r.removeFinalizer(ctx, pod))
		}

		if feature.VMIPRetainEnabled() {
			// TODO: use APIReader to get VM/VMI object, because watch v1.VirtualMachine and v1.VirtualMachineInstance will always get errors
			if isVMPod, vmName, _, err := strategy.OwnByVirtualMachine(ctx, ownedObj, r.APIReader); isVMPod {
				vm := &kubevirtv1.VirtualMachine{}
				if err = r.APIReader.Get(ctx, apitypes.NamespacedName{
					Name:      vmName,
					Namespace: pod.Namespace,
				}, vm); err != nil && !apierrors.IsNotFound(err) {
					return ctrl.Result{}, fmt.Errorf("failed to get vm %v: %v", vmName, err)
				}

				if apierrors.IsNotFound(err) || !vm.DeletionTimestamp.IsZero() {
					// if vm is deleted, should not reserve pod ips anymore
					return ctrl.Result{}, wrapError("unable to remove finalizer", r.removeFinalizer(ctx, pod))
				}

				// Before pod is not running, should not reserve ip instance because of pre-stop
				if !utils.PodIsNotRunning(pod) {
					return ctrl.Result{}, nil
				}

				log.V(1).Info("reserve ip for VM pod")
				if err = r.reserve(ctx, pod, types.DropPodName(true)); err != nil {
					return ctrl.Result{}, wrapError("unable to reserve pod", err)
				}

				vmIPInstances, err := utils.ListAllocatedIPInstances(ctx, r, client.MatchingLabels{
					constants.LabelVM: vmName,
				}, client.InNamespace(pod.Namespace))
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to list allocated ip instances for vm %v to reserve: %v",
						vmName, err)
				}

				for _, ipInstance := range vmIPInstances {
					if err := r.IPAMManager.Reserve(ipInstance.Spec.Network, []types.SubnetIPSuite{
						ipamtypes.ReserveIPOfSubnet(ipInstance.Spec.Subnet, utils.ToIPFormat(ipInstance.Name)),
					}); err != nil {
						return ctrl.Result{}, fmt.Errorf("failed to reserve ip %v for vm %v: %v",
							ipInstance.Spec.Address.IP, vmName, err)
					}
				}

				r.PodIPCache.ReleasePod(pod.Name, pod.Namespace)
				return ctrl.Result{}, wrapError("unable to remove finalizer", r.removeFinalizer(ctx, pod))
			} else if err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to check if pod %v/%v is for VM: %v", pod.Namespace, pod.Name, err)
			}
		}

		// For evicted and completed normal pods, pre decouple ip instances for completed or evicted pods
		if utils.PodIsEvicted(pod) || utils.PodIsCompleted(pod) {
			return ctrl.Result{}, wrapError("unable to decouple pod", r.decouple(ctx, pod))
		}

		return ctrl.Result{}, nil
	}

	// Unscheduled pods should not be processed
	if !utils.PodIsScheduled(pod) {
		return ctrl.Result{}, nil
	}

	var (
		networkStrFromWebhook  string
		subnetStrFromWebhook   string
		networkTypeFromWebhook types.NetworkType
		ipFamily               types.IPFamilyMode
	)

	var handledByWebhook = globalutils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationHandledByWebhook], false)
	// parse network and network-type in the webhook way
	if !handledByWebhook {
		if networkStrFromWebhook, subnetStrFromWebhook, networkTypeFromWebhook,
			ipFamily, _, _, err = utils.ParseNetworkConfigOfPodByPriority(ctx, r, pod); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to parse network config of pod: %v", err)
		}
	} else {
		ipFamily = ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])
	}

	cacheExist, uid, _ := r.PodIPCache.Get(pod.Name, pod.Namespace)
	// To avoid IP duplicate allocation
	if cacheExist && uid == pod.UID {
		return ctrl.Result{}, nil
	}

	networkName, err = r.selectNetwork(ctx, pod, handledByWebhook, networkStrFromWebhook, networkTypeFromWebhook)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to select network: %v", err)
	}

	if strategy.OwnByStatefulWorkload(pod) {
		log.V(1).Info("strategic allocation for stateful pod")
		return ctrl.Result{}, wrapError("unable to stateful allocate",
			r.statefulAllocate(ctx, pod, networkName, subnetStrFromWebhook, handledByWebhook, ipFamily))
	}

	if feature.VMIPRetainEnabled() {
		if isVMPod, vmName, vmiOwnerReference, err := strategy.OwnByVirtualMachine(ctx, pod, r.APIReader); isVMPod {
			log.V(1).Info("strategic allocation for VM pod")
			return ctrl.Result{}, wrapError("unable to vm allocate",
				r.vmAllocate(ctx, pod, vmName, networkName, subnetStrFromWebhook, handledByWebhook, vmiOwnerReference, ipFamily))
		} else if err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to check if pod %v/%v is for VM: %v", pod.Namespace, pod.Name, err)
		}
	}

	return ctrl.Result{}, wrapError("unable to allocate", r.allocate(ctx, pod, networkName,
		subnetStrFromWebhook, ipFamily, handledByWebhook))
}

// decouple will unbind IP instance with Pod
func (r *PodReconciler) decouple(ctx context.Context, pod *corev1.Pod) (err error) {
	if err = r.IPAMStore.DeCouple(ctx, pod); err != nil {
		return fmt.Errorf("unable to decouple ips for pod %s: %v", client.ObjectKeyFromObject(pod).String(), err)
	}

	r.Recorder.Event(pod, corev1.EventTypeNormal, ReasonIPReleaseSucceed, "pre decouple all IPs successfully")
	return nil
}

// reserve will reserve IP instances with Pod
func (r *PodReconciler) reserve(ctx context.Context, pod *corev1.Pod, reserveOptions ...types.ReserveOption) (err error) {
	if err = r.IPAMStore.IPReserve(ctx, pod, reserveOptions...); err != nil {
		return fmt.Errorf("unable to reserve ips for pod: %v", err)
	}

	r.Recorder.Event(pod, corev1.EventTypeNormal, ReasonIPReserveSucceed, "reserve all IPs successfully")
	return nil
}

// selectNetwork will pick the hit network by pod, taking the priority as below
// 1. explicitly specify network in pod annotations/labels
// 2. parse network type from pod and select a corresponding network binding on node
func (r *PodReconciler) selectNetwork(ctx context.Context, pod *corev1.Pod, handledByWebhook bool,
	networkStrFromWebhook string, networkTypeFromWebhook types.NetworkType) (string, error) {
	var specifiedNetwork string
	var networkType types.NetworkType
	var err error

	if !handledByWebhook {
		specifiedNetwork = networkStrFromWebhook
		networkType = networkTypeFromWebhook

		if len(specifiedNetwork) != 0 {
			// check network existence
			var (
				v1Network = &networkingv1.Network{}
				exist     bool
			)
			if exist, err = utils.CheckObjectExistence(ctx, r, apitypes.NamespacedName{Name: specifiedNetwork}, v1Network); err != nil {
				return "", fmt.Errorf("fail to check network existence: %v", err)
			}
			if !exist {
				return "", fmt.Errorf("specified network %s not found, should check webhook liveness", specifiedNetwork)
			}
		}

		// Should not return here, because wehook is missing and we need to make sure the pod
		// is scheduled to a node that matches the specified Network.
	} else if specifiedNetwork = globalutils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedNetwork],
		pod.Labels[constants.LabelSpecifiedNetwork]); len(specifiedNetwork) > 0 {
		return specifiedNetwork, nil
	} else {
		// Webhook is working, and no specified network for pod is found.
		networkType = types.ParseNetworkTypeFromString(globalutils.PickFirstNonEmptyString(
			pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType]))
	}

	var selectedNetworkName string
	switch networkType {
	case types.Underlay:
		// try to get underlay network by node name
		selectedNetworkName, err = r.getNetworkByNodeNameIndexer(ctx, pod.Spec.NodeName)
		if err != nil {
			return "", fmt.Errorf("unable to get underlay network by node name indexer: %v", err)
		}

		if len(selectedNetworkName) == 0 {
			return "", fmt.Errorf("unable to find underlay network for node %s, should check webhook liveness", pod.Spec.NodeName)
		}
	case types.Overlay:
		// try to get overlay network by special node name
		selectedNetworkName, err = r.getNetworkByNodeNameIndexer(ctx, OverlayNodeName)
		if err != nil {
			return "", fmt.Errorf("unable to get overlay network by node name indexer: %v", err)
		}

		if len(selectedNetworkName) == 0 {
			return "", fmt.Errorf("unable to find overlay network")
		}
	case types.GlobalBGP:
		// try to get global bgp network by special node name
		selectedNetworkName, err = r.getNetworkByNodeNameIndexer(ctx, GlobalBGPNodeName)
		if err != nil {
			return "", fmt.Errorf("unable to get overlay network by node name indexer: %v", err)
		}

		if len(selectedNetworkName) == 0 {
			return "", fmt.Errorf("unable to find global bgp network")
		}

		if !handledByWebhook {
			// check bgp attachment of node
			var node = &corev1.Node{}
			if err = r.Get(ctx, apitypes.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
				return "", fmt.Errorf("unalbe to get node %s: %v", pod.Spec.NodeName, err)
			}
			if _, attached := node.Labels[constants.LabelBGPNetworkAttachment]; !attached {
				return "", fmt.Errorf("node %s has not attached bgp network, should check webhook liveness", pod.Spec.NodeName)
			}
		}

	default:
		return "", fmt.Errorf("unknown network type %s from pod", networkType)
	}

	if !handledByWebhook {
		// check network selection conflict
		if len(specifiedNetwork) > 0 && specifiedNetwork != selectedNetworkName {
			return "", fmt.Errorf("different network selection between controller(%s) and webhook(%s), should check webhook liveness", selectedNetworkName, networkStrFromWebhook)
		}
	}

	return selectedNetworkName, nil
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

// statefulAllocate means an allocation on a stateful pod, including some
// special features, ip retain, ip reuse or ip assignment
func (r *PodReconciler) statefulAllocate(ctx context.Context, pod *corev1.Pod, networkName, subnetStrFromWebhook string,
	handledByWebhook bool, ipFamily types.IPFamilyMode) (err error) {
	var (
		shouldObserve = true
		startTime     = time.Now()
	)

	defer func() {
		if shouldObserve {
			metrics.IPAllocationPeriodSummary.
				WithLabelValues(metrics.IPStatefulAllocateType, strconv.FormatBool(err == nil)).
				Observe(float64(time.Since(startTime).Nanoseconds()))
		}
	}()

	// finalizer need to be added before ip allocation, because terminating pod without finalizer will not be reconciled
	if err = r.addFinalizer(ctx, pod); err != nil {
		return wrapError("unable to add finalizer for stateful pod", err)
	}

	// preAssign means that user want to assign some IPs to pod through annotation
	var preAssign = len(pod.Annotations[constants.AnnotationIPPool]) > 0

	// parse specified MAC address option from mac-pool annotation
	var specifiedMACAddr ipamtypes.SpecifiedMACAddress
	if len(pod.Annotations[constants.AnnotationMACPool]) > 0 {
		if specifiedMACAddr, err = parseSpecifiedMACAddressOption(pod); err != nil {
			return wrapError("fail to parse specified MAC address option for stateful pod", err)
		}

		// check specified MAC address collision in advance
		if err = r.checkMACAddressCollision(pod, networkName, string(specifiedMACAddr)); err != nil {
			return wrapError("fail to check specified MAC address collision: %v", err)
		}
	}

	// expectReallocate means that ip is expected to be released and allocated again, usually
	// this will be set true when ip is leaking
	// 1. global retain and pod retain or unset, ip should be retained
	// 2. global retain and pod not retain, ip should be reallocated
	// 3. global not retain and pod not retain or unset, ip should be reallocated
	// 4. global not retain and pod retain, ip should be retained
	var expectReallocate = !globalutils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationIPRetain], strategy.DefaultIPRetain)

	// shouldReallocate means that ip should be released and allocated again
	// if pre-assigned through annotation, this must be false
	var shouldReallocate = expectReallocate && !preAssign

	if shouldReallocate {
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

		return wrapError("unable to reallocate", r.allocate(ctx, pod, networkName,
			subnetStrFromWebhook, ipFamily, handledByWebhook, specifiedMACAddr))
	}

	var (
		ipCandidates []ipCandidate
		forceAssign  = false
	)
	if preAssign {
		ipPool := strings.Split(pod.Annotations[constants.AnnotationIPPool], ",")
		idx := utils.GetIndexFromName(pod.Name)

		if idx >= len(ipPool) {
			return fmt.Errorf("unable to find assigned ip in ip-pool %s by index %d", pod.Annotations[constants.AnnotationIPPool], idx)
		}

		if len(ipPool[idx]) == 0 {
			return fmt.Errorf("the %d assigned ip section is empty in ip-pool %s", idx, pod.Annotations[constants.AnnotationIPPool])
		}

		for _, ipStr := range strings.Split(ipPool[idx], "/") {
			normalizedIP := globalutils.NormalizedIP(ipStr)
			if len(normalizedIP) == 0 {
				return fmt.Errorf("the assigned ip %s is illegal", ipStr)
			}

			// pre assignment only have IP
			ipCandidates = append(ipCandidates, ipCandidate{
				ip: normalizedIP,
			})
		}
		// pre assignment can force using reserved IPs
		forceAssign = true
	} else {
		var allocatedIPInstances []*networkingv1.IPInstance
		if allocatedIPInstances, err = utils.ListAllocatedIPInstancesOfPod(ctx, r, pod); err != nil {
			return err
		}

		// allocated reuse will have both subnet and IP, also IP candidates should follow
		// ip family order, ipv4 before ipv6
		networkingv1.SortIPInstancePointerSlice(allocatedIPInstances)
		for i := range allocatedIPInstances {
			var ipInstance = allocatedIPInstances[i]
			ipCandidates = append(ipCandidates, ipCandidate{
				subnet: ipInstance.Spec.Subnet,
				ip:     utils.ToIPFormat(ipInstance.Name),
			})
		}

		// when no valid ip found, it means that this is the first time of pod creation
		if len(ipCandidates) == 0 {
			// allocate has its own observation process, so just skip
			shouldObserve = false
			return wrapError("unable to allocate", r.allocate(ctx, pod, networkName, subnetStrFromWebhook,
				ipFamily, handledByWebhook, specifiedMACAddr))
		}
	}

	// assign IP candidates to pod
	return wrapError("unable to assign", r.assign(ctx, pod, networkName, ipCandidates, forceAssign,
		ipFamily, specifiedMACAddr))
}

func (r *PodReconciler) vmAllocate(ctx context.Context, pod *corev1.Pod, vmName, networkName, subnetStrFromWebhook string,
	handledByWebhook bool, vmiOwnerReference *metav1.OwnerReference, ipFamily types.IPFamilyMode) (err error) {
	// finalizer need to be added before ip allocation, because terminating pod without finalizer will not be reconciled
	if err = r.addFinalizer(ctx, pod); err != nil {
		return wrapError("unable to add finalizer for vm pod", err)
	}

	vmLabels := client.MatchingLabels{
		constants.LabelVM: vmName,
	}

	var ipCandidates []ipCandidate
	var allocatedIPInstances []*networkingv1.IPInstance

	if allocatedIPInstances, err = utils.ListAllocatedIPInstances(ctx, r, vmLabels,
		client.InNamespace(pod.Namespace)); err != nil {
		return fmt.Errorf("failed to list allocated ip instances for vm %v: %v", vmName, err)
	}

	// allocated reuse will have both subnet and IP, also IP candidates should follow
	// ip family order, ipv4 before ipv6
	networkingv1.SortIPInstancePointerSlice(allocatedIPInstances)
	for i := range allocatedIPInstances {
		var ipInstance = allocatedIPInstances[i]
		ipCandidates = append(ipCandidates, ipCandidate{
			subnet: ipInstance.Spec.Subnet,
			ip:     utils.ToIPFormat(ipInstance.Name),
		})
	}

	// when no valid ip found, it means that this is the first time of pod creation
	if len(ipCandidates) == 0 {
		return wrapError("unable to allocate", r.allocate(ctx, pod, networkName, subnetStrFromWebhook, ipFamily,
			handledByWebhook, types.AdditionalLabels(vmLabels), types.OwnerReference(*vmiOwnerReference)))
	}

	// forced assign for using reserved ips
	return wrapError("unable to multi-assign", r.assign(ctx, pod, networkName, ipCandidates, true, ipFamily,
		types.AdditionalLabels(vmLabels), types.OwnerReference(*vmiOwnerReference)))
}

// recycleNonCandidateReservedIPs recycles IPs that should NO LONGER serve given pod, i.e., release
// reserved IPs that does not appear in candidates.
func (r *PodReconciler) recycleNonCandidateReservedIPs(ctx context.Context, pod *corev1.Pod, ipCandidates []ipCandidate) (err error) {
	var (
		allocatedIPs []*networkingv1.IPInstance
		toReleaseIPs []*networkingv1.IPInstance
		toStayIPs    = sets.NewString()
	)
	for _, candidate := range ipCandidates {
		toStayIPs.Insert(candidate.ip)
	}
	if allocatedIPs, err = utils.ListAllocatedIPInstancesOfPod(ctx, r, pod); err != nil {
		return fmt.Errorf("failed to list allocated ip instances for pod %v/%v: %v", pod.Namespace, pod.Name, err)
	}
	for i := range allocatedIPs {
		if toStayIPs.Has(string(globalutils.StringToIPNet(allocatedIPs[i].Spec.Address.IP).IP)) {
			continue
		}
		toReleaseIPs = append(toReleaseIPs, allocatedIPs[i])
	}
	if len(toReleaseIPs) > 0 {
		err = r.release(ctx, pod, transform.TransferIPInstancesForIPAM(toReleaseIPs))
		if err != nil {
			return wrapError("failed to release ips that no longer serve current pod", err)
		}
	}
	return nil
}

// assign means some allocated or pre-assigned IPs will be assigned to a specified pod
func (r *PodReconciler) assign(ctx context.Context, pod *corev1.Pod, networkName string, ipCandidates []ipCandidate, force bool,
	ipFamily types.IPFamilyMode, reCoupleOptions ...types.ReCoupleOption) (err error) {
	if force {
		// recycle non-candidate reserved IPs, as pre-assigned IPs for current pod
		// could be changed.
		if err = r.recycleNonCandidateReservedIPs(ctx, pod, ipCandidates); err != nil {
			return
		}
	}

	// try to assign candidate IPs to pod
	var AssignedIPs []*types.IP
	if AssignedIPs, err = r.IPAMManager.Assign(networkName,
		ipamtypes.PodInfo{
			NamespacedName: apitypes.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
			IPFamily: ipFamily,
		},
		ipCandidateToAssignSuite(ipCandidates),
		ipamtypes.AssignForce(force),
	); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = r.IPAMManager.Release(networkName, ipToReleaseSuite(AssignedIPs))
		}
	}()

	if err = r.IPAMStore.ReCouple(ctx, pod, AssignedIPs, reCoupleOptions...); err != nil {
		return fmt.Errorf("fail to force-couple IPs %+v with pod: %v", AssignedIPs, err)
	}

	// always keep updating pod ip cache the final step
	r.PodIPCache.Record(pod.UID, pod.Name, pod.Namespace, ipToIPInstanceName(AssignedIPs))

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPAllocationSucceed, "assign IPs %v successfully", ipToIPString(AssignedIPs))
	return nil
}

// release will release IP instances of pod
func (r *PodReconciler) release(ctx context.Context, pod *corev1.Pod, allocatedIPs []*types.IP) (err error) {
	for _, ip := range allocatedIPs {
		if err = r.IPAMStore.IPRecycle(ctx, pod.Namespace, ip); err != nil {
			return fmt.Errorf("unable to recycle ip %v: %v", ip, err)
		}
	}

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPReleaseSucceed, "release IPs %v successfully", ipToIPString(allocatedIPs))
	return nil
}

// allocate will allocate new IPs for pod
func (r *PodReconciler) allocate(ctx context.Context, pod *corev1.Pod, networkName, subnetStrFromWebhook string,
	ipFamily types.IPFamilyMode, handledByWebhook bool, coupleOptions ...types.CoupleOption) (err error) {
	var startTime = time.Now()
	defer func() {
		metrics.IPAllocationPeriodSummary.
			WithLabelValues(metrics.IPNormalAllocateType, strconv.FormatBool(err == nil)).
			Observe(float64(time.Since(startTime).Nanoseconds()))
	}()

	var (
		subnetNameStr        string
		specifiedSubnetNames []string
		allocatedIPs         []*types.IP
	)

	if !handledByWebhook {
		subnetNameStr = subnetStrFromWebhook
	} else {
		subnetNameStr = globalutils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet],
			pod.Labels[constants.LabelSpecifiedSubnet])
	}

	if len(subnetNameStr) > 0 {
		specifiedSubnetNames = strings.Split(subnetNameStr, "/")
	}

	if allocatedIPs, err = r.IPAMManager.Allocate(networkName, ipamtypes.PodInfo{
		NamespacedName: apitypes.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		},
		IPFamily: ipFamily,
	}, ipamtypes.AllocateSubnets(specifiedSubnetNames)); err != nil {
		return fmt.Errorf("unable to allocate IP on family %s : %v", ipFamily, err)
	}

	defer func() {
		if err != nil {
			_ = r.IPAMManager.Release(networkName, ipToReleaseSuite(allocatedIPs))
		}
	}()

	if err = r.IPAMStore.Couple(ctx, pod, allocatedIPs, coupleOptions...); err != nil {
		return fmt.Errorf("unable to couple IPs %v with pod: %v", allocatedIPs, err)
	}

	// Always keep updating pod ip cache the final step.
	r.PodIPCache.Record(pod.UID, pod.Name, pod.Namespace, ipToIPInstanceName(allocatedIPs))

	r.Recorder.Eventf(pod, corev1.EventTypeNormal, ReasonIPAllocationSucceed, "allocate IPs %v successfully", ipToIPString(allocatedIPs))
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

func (r *PodReconciler) checkMACAddressCollision(pod *corev1.Pod, networkName string, macAddr string) (err error) {
	ipInstanceList := &networkingv1.IPInstanceList{}
	if err = r.List(context.TODO(), ipInstanceList,
		client.MatchingLabels{constants.LabelNetwork: networkName},
		client.MatchingFields{IndexerFieldMAC: macAddr}); err != nil {
		return fmt.Errorf("unable to list ip instances by indexer MAC %s: %v", macAddr, err)
	}
	for _, ipInstance := range ipInstanceList.Items {
		if !ipInstance.DeletionTimestamp.IsZero() {
			continue
		}
		if ipInstance.Namespace != pod.GetNamespace() || ipInstance.Spec.Binding.PodName != pod.GetName() {
			return fmt.Errorf("specified mac address %s is in conflict with existing ip instance %s/%s", macAddr, ipInstance.Namespace, ipInstance.Name)
		}
	}
	return nil
}

func ipToReleaseSuite(ips []*types.IP) (ret []ipamtypes.SubnetIPSuite) {
	for _, ip := range ips {
		ret = append(ret, ipamtypes.ReleaseIPOfSubnet(ip.Subnet, ip.Address.IP.String()))
	}
	return
}

func ipToIPInstanceName(ips []*types.IP) (ret []string) {
	for _, ip := range ips {
		ret = append(ret, utils.ToDNSFormat(ip.Address.IP))
	}
	return
}

func ipToIPString(ips []*types.IP) (ret []string) {
	for _, ip := range ips {
		ret = append(ret, ip.Address.IP.String())
	}
	return
}

func ipCandidateToAssignSuite(ipCandidates []ipCandidate) (ret []types.SubnetIPSuite) {
	for _, ipCandidate := range ipCandidates {
		if len(ipCandidate.subnet) == 0 {
			ret = append(ret, ipamtypes.AssignIP(ipCandidate.ip))
		} else {
			ret = append(ret, ipamtypes.AssignIPOfSubnet(ipCandidate.subnet, ipCandidate.ip))
		}
	}
	return
}

func parseSpecifiedMACAddressOption(pod *corev1.Pod) (mac ipamtypes.SpecifiedMACAddress, err error) {
	if len(pod.Annotations[constants.AnnotationMACPool]) == 0 {
		return "", nil
	}

	macPool := strings.Split(pod.Annotations[constants.AnnotationMACPool], ",")
	idx := utils.GetIndexFromName(pod.Name)

	if idx >= len(macPool) {
		return "", fmt.Errorf("unable to find assigned mac address in mac pool %s by index %d", pod.Annotations[constants.AnnotationMACPool], idx)
	}

	if len(macPool[idx]) == 0 {
		return "", fmt.Errorf("the %d assigned mac address is empty in mac pool %s", idx, pod.Annotations[constants.AnnotationMACPool])
	}

	normalizedMAC := macutils.NormalizeMAC(macPool[idx])
	if len(normalizedMAC) == 0 {
		return "", fmt.Errorf("the assigned mac address %s is illegal", macPool[idx])
	}

	return ipamtypes.SpecifiedMACAddress(normalizedMAC), nil
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
					if r.PodSelector == nil {
						return true
					}

					pod, ok := obj.(*corev1.Pod)
					if !ok {
						return false
					}

					// Only selected pods should be processed
					return r.PodSelector.Matches(pod)
				}),
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
						return utils.PodIsScheduled(pod)
					}

					// Terminating pods ip-allocated finalizer should be processed specially.
					// Pods without ip-allocated finalizer will be considered as having no retained ip.
					return controllerutil.ContainsFinalizer(pod, constants.FinalizerIPAllocated)
				}),
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Max(),
			RecoverPanic:            true,
		}).
		Complete(r)
}
