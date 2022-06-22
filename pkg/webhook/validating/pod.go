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

package validating

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"

	webhookutils "github.com/alibaba/hybridnet/pkg/webhook/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils"
)

var podGVK = gvkConverter(corev1.SchemeGroupVersion.WithKind("Pod"))

func init() {
	createHandlers[podGVK] = PodCreateValidation
}

func PodCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	err := handler.Decoder.Decode(*req, pod)
	if err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	if pod.Spec.HostNetwork {
		return admission.Allowed("skip validation on host-networking pod")
	}

	var (
		networkTypeFromPod = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType])
		networkType        = ipamtypes.ParseNetworkTypeFromString(networkTypeFromPod)
	)

	specifiedNetwork, specifiedSubnetStr, err := webhookutils.SelectNetworkAndSubnetFromObject(ctx, handler.Cache, pod)
	if err != nil {
		return webhookutils.AdmissionDeniedWithLog(err.Error(), logger)
	}

	if len(specifiedNetwork) > 0 {
		// Specified Network Validation
		network := &networkingv1.Network{}
		if err = handler.Cache.Get(ctx, types.NamespacedName{Name: specifiedNetwork}, network); err != nil {
			if errors.IsNotFound(err) {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("specified network %s not found", specifiedNetwork), logger)
			}
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}

		// check network type, if network type is explicitly specified on pod, use it for comparison directly,
		// or else mutate network type inherit from specified network
		if len(networkTypeFromPod) > 0 {
			if !stringEqualCaseInsensitive(string(networkingv1.GetNetworkType(network)), string(networkType)) {
				return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError,
					fmt.Errorf("specified network type mismatch, network-type %s, network %s", networkType, network.Name), logger)
			}
		} else {
			networkType = ipamtypes.ParseNetworkTypeFromString(string(networkingv1.GetNetworkType(network)))
		}

		// Existing IP Instances Validation
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

		for i := range ipList.Items {
			var ipInstance = &ipList.Items[i]
			// terminating ipInstance should be ignored
			if ipInstance.DeletionTimestamp == nil {
				switch {
				case ipInstance.Spec.Network != specifiedNetwork:
					return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf(
						"pod has assigned ip %s of network %s, cannot assign to another network %s",
						ipInstance.Spec.Address.IP,
						ipInstance.Spec.Network,
						specifiedNetwork,
					), logger)
				case len(specifiedSubnetStr) > 0 && !webhookutils.SubnetNameBelongsToSpecifiedSubnets(ipInstance.Spec.Subnet, specifiedSubnetStr):
					return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf(
						"pod has assigend ip %s of subnet %s, cannot assign to another subnet by specified string %s",
						ipInstance.Spec.Address.IP,
						ipInstance.Spec.Subnet,
						specifiedSubnetStr,
					), logger)
				}
			}
		}
	}

	// IP Pool Validation
	var ipPool string
	if ipPool = pod.Annotations[constants.AnnotationIPPool]; len(ipPool) > 0 {
		if len(specifiedNetwork) == 0 {
			return webhookutils.AdmissionDeniedWithLog("ip pool and network(subnet) must be specified at the same time", logger)
		}
		ips := strings.Split(ipPool, ",")
		for idx, ipSegment := range ips {
			if len(ipSegment) == 0 {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("the %d ip in ip pool is empty", idx), logger)
			}

			// if dual stack IP family, more than one IP should be assigned
			for _, ip := range strings.Split(ipPool, "/") {
				if utils.NormalizedIP(ip) != ip {
					return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("ip pool has an invalid ip %s", ip), logger)
				}
			}
		}
	}

	// Network type validation
	switch networkType {
	case ipamtypes.Underlay, ipamtypes.Overlay, ipamtypes.GlobalBGP:
	default:
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("unrecognized network type %s", networkType), logger)
	}

	// IP family validation
	var ipFamily = ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])
	switch ipFamily {
	case ipamtypes.IPv4, ipamtypes.IPv6, ipamtypes.DualStack:
	default:
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("unrecognized ip family %s", ipFamily), logger)
	}

	// Network availability validation
	// For underlay network type, pod will be patched some quota labels when mutating to be scheduled on nodes which
	// have available underlay network
	// For other network types (now only some global unique network types), pod will be denied here if network is
	// not available
	var networkTypeInSpec = networkingv1.NetworkType(networkType)
	switch networkTypeInSpec {
	case networkingv1.NetworkTypeOverlay, networkingv1.NetworkTypeGlobalBGP:
		networkList := &networkingv1.NetworkList{}
		if err = handler.Client.List(ctx, networkList); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}

		idx := -1
		for i := range networkList.Items {
			if networkingv1.GetNetworkType(&networkList.Items[i]) == networkTypeInSpec {
				idx = i
				break
			}
		}

		if idx < 0 {
			return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("no network found by type %s", networkTypeInSpec), logger)
		}

		network := &networkList.Items[idx]

		switch ipFamily {
		case ipamtypes.IPv4:
			if !networkingv1.IsAvailable(network.Status.Statistics) {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("lacking ipv4 addresses by network type %s", networkTypeInSpec), logger)
			}
		case ipamtypes.IPv6:
			if !networkingv1.IsAvailable(network.Status.IPv6Statistics) {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("lacking ipv6 addresses by network type %s", networkTypeInSpec), logger)
			}
		case ipamtypes.DualStack:
			if !networkingv1.IsAvailable(network.Status.DualStackStatistics) {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("lacking dual stack addresses by network type %s", networkTypeInSpec), logger)
			}
		}
	}

	return admission.Allowed("validation pass")
}

func stringEqualCaseInsensitive(a, b string) bool {
	return strings.EqualFold(a, b)
}
