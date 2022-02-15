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

	"github.com/go-logr/logr"

	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils"
	webhookutils "github.com/alibaba/hybridnet/pkg/webhook/utils"
)

// TaintNodeNetworkUnavailable will be added when node's network is unavailable
// and removed when network becomes ready.
const TaintNodeNetworkUnavailable = "node.kubernetes.io/network-unavailable"

var podGVK = corev1.SchemeGroupVersion.WithKind("Pod")

func init() {
	createHandlers[gvkConverter(podGVK)] = PodCreateMutation
}

func PodCreateMutation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	err := handler.Decoder.Decode(*req, pod)
	if err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	if pod.Spec.HostNetwork {
		// make sure host-networking pod will not be affected by taint from hybridnet
		return generatePatchResponseFromPod(req.Object.Raw, ensureTolerationInPod(pod,
			&corev1.Toleration{
				Key:      TaintNodeNetworkUnavailable,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			}), logger)
	}

	// Remove unexpected annotation
	// 1. pod IP annotation
	if _, exist := pod.Annotations[constants.AnnotationIP]; exist {
		logger.Info("remove IP annotation for pod", "namespace", req.Namespace, "name", req.Name)
		delete(pod.Annotations, constants.AnnotationIP)
	}

	// Select specific network for pod
	// Priority as below
	// 1. Network & Subnet from Pod
	// 2. Network & Subnet from Namespace
	// 3. Allocated IP Instance for Pod in Stateful Workloads
	var (
		networkName          string
		subnetNameStr        string
		networkNameFromPod   string
		subnetNameStrFromPod string
		networkNameFromNs    string
		subnetNameStrFromNs  string
	)

	if networkNameFromPod, subnetNameStrFromPod, err = webhookutils.SelectNetworkAndSubnetFromObject(ctx, handler.Cache, pod); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest,
			fmt.Errorf("failed to select network and subnet for pod %s/%s: %v", req.Name, req.Namespace, err), logger)
	}

	ns := &corev1.Namespace{}
	// pod.Name/pod.Namespace/req.Name are empty string
	if err = handler.Cache.Get(ctx, types.NamespacedName{Name: req.Namespace}, ns); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError,
			fmt.Errorf("failed to get namespace for pod %s/%s: %v", req.Name, req.Namespace, err), logger)
	}
	if networkNameFromNs, subnetNameStrFromNs, err = webhookutils.SelectNetworkAndSubnetFromObject(ctx, handler.Cache, ns); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest,
			fmt.Errorf("failed to select network and subnet for ns %s: %v", ns.Name, err), logger)
	}

	switch {
	case len(networkNameFromPod) > 0:
		networkName = networkNameFromPod
		switch {
		case len(subnetNameStrFromPod) > 0:
			subnetNameStr = subnetNameStrFromPod
		case networkNameFromPod == networkNameFromNs:
			// if network match between pod and ns, subnet from ns
			// will be referred
			subnetNameStr = subnetNameStrFromNs
		}
	case len(networkNameFromNs) > 0:
		networkName = networkNameFromNs
		subnetNameStr = subnetNameStrFromNs
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
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)

		}

		// ignore terminating ipInstance
		for i := range ipList.Items {
			if ipList.Items[i].DeletionTimestamp == nil {
				networkName = ipList.Items[i].Spec.Network
				break
			}
		}
	}

	var networkTypeFromNs = utils.PickFirstNonEmptyString(ns.GetAnnotations()[constants.AnnotationNetworkType],
		ns.GetLabels()[constants.LabelNetworkType])
	var networkTypeFromPod = utils.PickFirstNonEmptyString(pod.GetAnnotations()[constants.AnnotationNetworkType],
		pod.GetLabels()[constants.LabelNetworkType])

	if len(networkTypeFromPod) == 0 {
		networkTypeFromPod = networkTypeFromNs
	}

	var ipFamilyFromNs = ns.GetAnnotations()[constants.AnnotationIPFamily]
	var ipFamilyFromPod = pod.GetAnnotations()[constants.AnnotationIPFamily]

	if len(ipFamilyFromPod) == 0 {
		ipFamilyFromPod = ipFamilyFromNs
	}

	var networkType = ipamtypes.ParseNetworkTypeFromString(networkTypeFromPod)
	var networkNodeSelector map[string]string
	if len(networkName) > 0 {
		network := &networkingv1.Network{}
		if err = handler.Client.Get(ctx, types.NamespacedName{Name: networkName}, network); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}

		// specified network takes higher priority than network type defaulting, if no network type specified
		// from pod, then network type should inherit from network type of specified network from pod
		if len(networkTypeFromPod) == 0 {
			networkType = ipamtypes.ParseNetworkTypeFromString(string(networkingv1.GetNetworkType(network)))
		}

		networkNodeSelector = network.Spec.NodeSelector
	}

	// persistent specified network and subnet in pod annotations
	patchAnnotationToPod(pod, constants.AnnotationSpecifiedNetwork, networkName)
	patchAnnotationToPod(pod, constants.AnnotationSpecifiedSubnet, subnetNameStr)
	patchAnnotationToPod(pod, constants.AnnotationNetworkType, string(networkType))
	patchAnnotationToPod(pod, constants.AnnotationIPFamily, ipFamilyFromPod)

	switch networkType {
	case ipamtypes.Underlay:
		if len(networkName) > 0 {
			logger.Info("patch pod with selector of network",
				"namespace", req.Namespace, "name", req.Name, "network", networkName)
			patchSelectorToPod(pod, networkNodeSelector)
		} else {
			logger.Info("patch pod with underlay attachment selector",
				"namespace", req.Namespace, "name", req.Name)
			patchSelectorToPod(pod, map[string]string{
				constants.LabelUnderlayNetworkAttachment: constants.Attached,
			})
		}
		// quota label selector to make sure pod will be scheduled on nodes
		// where capacity of network is enough
		if feature.DualStackEnabled() {
			switch ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily]) {
			case ipamtypes.IPv4Only:
				patchSelectorToPod(pod, map[string]string{
					constants.LabelIPv4AddressQuota: constants.QuotaNonEmpty,
				})
			case ipamtypes.IPv6Only:
				patchSelectorToPod(pod, map[string]string{
					constants.LabelIPv6AddressQuota: constants.QuotaNonEmpty,
				})
			case ipamtypes.DualStack:
				patchSelectorToPod(pod, map[string]string{
					constants.LabelDualStackAddressQuota: constants.QuotaNonEmpty,
				})
			}
		} else {
			patchSelectorToPod(pod, map[string]string{
				constants.LabelAddressQuota: constants.QuotaNonEmpty,
			})
		}
	case ipamtypes.Overlay:
		logger.Info("patch pod with overlay attachment selector",
			"namespace", req.Namespace, "name", req.Name)
		patchSelectorToPod(pod, map[string]string{
			constants.LabelOverlayNetworkAttachment: constants.Attached,
		})
	default:
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, fmt.Errorf("unknown network type %s", networkType), logger)
	}

	return generatePatchResponseFromPod(req.Object.Raw, pod, logger)
}

func generatePatchResponseFromPod(original []byte, pod *corev1.Pod, logger logr.Logger) admission.Response {
	marshaled, err := json.Marshal(pod)
	if err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
	}

	return admission.PatchResponseFromRaw(original, marshaled)
}

func patchSelectorToPod(pod *corev1.Pod, selector map[string]string) {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = selector
		return
	}

	for k, v := range selector {
		pod.Spec.NodeSelector[k] = v
	}
}

func patchAnnotationToPod(pod *corev1.Pod, key, value string) {
	if len(value) == 0 {
		return
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{
			key: value,
		}
		return
	}

	pod.Annotations[key] = value
}

func ensureTolerationInPod(pod *corev1.Pod, tolerations ...*corev1.Toleration) *corev1.Pod {
	for _, toleration := range tolerations {
		var found = false
		for i := range pod.Spec.Tolerations {
			found = found || tolerationMatch(&pod.Spec.Tolerations[i], toleration)
		}
		if !found {
			pod.Spec.Tolerations = append(pod.Spec.Tolerations, *toleration)
		}
	}

	return pod
}

func tolerationMatch(orig, diff *corev1.Toleration) bool {
	return orig.Key == diff.Key &&
		orig.Effect == diff.Effect &&
		orig.Operator == diff.Operator &&
		orig.Value == diff.Value
}
