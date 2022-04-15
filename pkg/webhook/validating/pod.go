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
	"github.com/alibaba/hybridnet/pkg/feature"
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
		for _, ip := range ips {
			if utils.NormalizedIP(ip) != ip {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("ip pool has invalid ip %s", ip), logger)
			}
		}
	}

	// Overlay network capacity validation
	if feature.DualStackEnabled() && networkType == ipamtypes.Overlay {
		networkList := &networkingv1.NetworkList{}
		if err = handler.Client.List(ctx, networkList); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}
		for i := range networkList.Items {
			network := &networkList.Items[i]
			if network.Spec.Type == networkingv1.NetworkTypeOverlay {
				switch ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily]) {
				case ipamtypes.IPv4Only:
					if network.Status.Statistics.Available <= 0 {
						return webhookutils.AdmissionDeniedWithLog("lacking ipv4 addresses for overlay mode", logger)
					}
				case ipamtypes.IPv6Only:
					if network.Status.IPv6Statistics.Available <= 0 {
						return webhookutils.AdmissionDeniedWithLog("lacking ipv6 addresses for overlay mode", logger)
					}
				case ipamtypes.DualStack:
					if network.Status.DualStackStatistics.Available <= 0 {
						return webhookutils.AdmissionDeniedWithLog("lacking dual stack addresses for overlay mode", logger)
					}
				}
			}
		}
	}

	return admission.Allowed("validation pass")
}

func stringEqualCaseInsensitive(a, b string) bool {
	return strings.EqualFold(a, b)
}
