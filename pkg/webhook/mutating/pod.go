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

package mutating

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils"
)

// Copy from latest k8s
// TaintNodeNetworkUnavailable will be added when node's network is unavailable
// and removed when network becomes ready.
const TaintNodeNetworkUnavailable = "node.kubernetes.io/network-unavailable"

var podGVK = corev1.SchemeGroupVersion.WithKind("Pod")

func init() {
	createHandlers[gvkConverter(podGVK)] = PodCreateMutation
}

func PodCreateMutation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	pod := &corev1.Pod{}
	err := handler.Decoder.Decode(*req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if pod.Spec.HostNetwork {
		// make sure host-networking pod will not be affected by taint from hybridnet
		return generatePatchResponseFromPod(req.Object.Raw, ensureTolerationInPod(pod, &corev1.Toleration{
			Key:      TaintNodeNetworkUnavailable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		}))
	}

	var mutated = false

	// Remove unexpected annotation
	// 1. pod IP annotation
	if _, exist := pod.Annotations[constants.AnnotationIP]; exist {
		klog.Infof("[mutating] remove IP annotation for pod %s/%s", req.Namespace, req.Name)
		delete(pod.Annotations, constants.AnnotationIP)
		mutated = true
	}

	// Select specific network for pod
	// Priority as below
	// 1. Network Annotation/Label from Pod
	// 2. Allocated IP Instance for Pod in Stateful Workloads
	var (
		networkName        string
		networkNameFromPod = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedNetwork], pod.Labels[constants.LabelSpecifiedNetwork])
	)
	switch {
	case len(networkNameFromPod) > 0:
		networkName = networkNameFromPod
	case strategy.OwnByStatefulWorkload(pod):
		if shouldReallocate := !utils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationIPRetain], strategy.DefaultIPRetain); shouldReallocate {
			// reallocate means that pod will locate on node freely
			break
		}

		ipList := &networkingv1.IPInstanceList{}
		if err = handler.Client.List(
			ctx,
			ipList,
			client.InNamespace(pod.Namespace),
			client.MatchingLabels{
				constants.LabelPod: pod.Name,
			}); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)

		}

		// ignore terminating ipInstance
		for i := range ipList.Items {
			if ipList.Items[i].DeletionTimestamp == nil {
				networkName = ipList.Items[i].Spec.Network
				break
			}
		}
	}

	var networkTypeFromPod = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType])
	var networkType = ipamtypes.ParseNetworkTypeFromString(networkTypeFromPod)
	if len(networkName) > 0 {
		network := &networkingv1.Network{}
		if err = handler.Client.Get(ctx, types.NamespacedName{Name: networkName}, network); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// specified network takes higher priority than network type defaulting, if no network type specified
		// from pod, then network type should inherit from network type of specified network from pod
		if len(networkTypeFromPod) == 0 {
			networkType = ipamtypes.ParseNetworkTypeFromString(string(networkingv1.GetNetworkType(network)))
		}

		switch networkType {
		case ipamtypes.Underlay:
			klog.Infof("[mutating] patch pod %s/%s with selector of network %s", req.Namespace, req.Name, networkName)
			pod = patchSelectorToPod(pod, network.Spec.NodeSelector)
			mutated = true
		case ipamtypes.Overlay:
			// overlay network has a wide scheduling domain over the whole cluster
			// no more selectors should be patched
		default:
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("unknown network type %s", networkType))
		}
	}

	// Patch extra node selectors for pod when dual stack mode,
	// capacity check of overlay network is in validation process, so extra
	// label selectors is not required
	if feature.DualStackEnabled() && networkType == ipamtypes.Underlay {
		switch ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily]) {
		case ipamtypes.IPv4Only:
			pod = patchSelectorToPod(pod, map[string]string{
				constants.LabelIPv4AddressQuota: constants.QuotaNonEmpty,
			})
		case ipamtypes.IPv6Only:
			pod = patchSelectorToPod(pod, map[string]string{
				constants.LabelIPv6AddressQuota: constants.QuotaNonEmpty,
			})
		case ipamtypes.DualStack:
			pod = patchSelectorToPod(pod, map[string]string{
				constants.LabelDualStackAddressQuota: constants.QuotaNonEmpty,
			})
		}
		mutated = true
	}

	if mutated {
		return generatePatchResponseFromPod(req.Object.Raw, pod)
	}

	return admission.Patched("no need to patch without specific network")
}

func generatePatchResponseFromPod(original []byte, pod *corev1.Pod) admission.Response {
	marshaled, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(original, marshaled)
}

func patchSelectorToPod(pod *corev1.Pod, selector map[string]string) *corev1.Pod {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = selector
		return pod
	}

	for k, v := range selector {
		pod.Spec.NodeSelector[k] = v
	}

	return pod
}

func ensureTolerationInPod(pod *corev1.Pod, toleration *corev1.Toleration) *corev1.Pod {
	for i := range pod.Spec.Tolerations {
		if tolerationMatch(&pod.Spec.Tolerations[i], toleration) {
			return pod
		}
	}

	pod.Spec.Tolerations = append(pod.Spec.Tolerations, *toleration)
	return pod
}

func tolerationMatch(orig, diff *corev1.Toleration) bool {
	return orig.Key == diff.Key &&
		orig.Effect == diff.Effect &&
		orig.Operator == diff.Operator &&
		orig.Value == diff.Value
}
