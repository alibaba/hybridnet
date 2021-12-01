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
	pod := &corev1.Pod{}
	err := handler.Decoder.Decode(*req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Specified Network Validation
	var (
		specifiedNetwork   = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedNetwork], pod.Labels[constants.LabelSpecifiedNetwork])
		networkTypeFromPod = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType])
		networkType        = ipamtypes.ParseNetworkTypeFromString(networkTypeFromPod)
	)
	if len(specifiedNetwork) > 0 {
		network := &networkingv1.Network{}
		if err = handler.Cache.Get(ctx, types.NamespacedName{Name: specifiedNetwork}, network); err != nil {
			if errors.IsNotFound(err) {
				return admission.Denied(fmt.Sprintf("specified network %s not found", specifiedNetwork))
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// check network type, if network type is explicitly specified on pod, use it for comparison directly,
		// or else mutate network type inherit from specified network
		if len(networkTypeFromPod) > 0 {
			if !stringEqualCaseInsensitive(string(networkingv1.GetNetworkType(network)), string(networkType)) {
				return admission.Errored(http.StatusInternalServerError, fmt.Errorf("specified network type mismatch %s %s", networkType, networkingv1.GetNetworkType(network)))
			}
		} else {
			networkType = ipamtypes.ParseNetworkTypeFromString(string(networkingv1.GetNetworkType(network)))
		}
	}

	// Specified Subnet Validation
	var specifiedSubnet string
	if specifiedSubnet = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet], pod.Labels[constants.LabelSpecifiedSubnet]); len(specifiedSubnet) > 0 {
		subnet := &networkingv1.Subnet{}
		if err = handler.Cache.Get(ctx, types.NamespacedName{Name: specifiedSubnet}, subnet); err != nil {
			if errors.IsNotFound(err) {
				return admission.Denied(fmt.Sprintf("specified subnet %s not found", specifiedSubnet))
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}

		if len(specifiedNetwork) > 0 {
			if subnet.Spec.Network != specifiedNetwork {
				return admission.Denied(fmt.Sprintf("specified subnet %s not belong to specified network %s",
					specifiedSubnet,
					specifiedNetwork,
				))
			}
		} else {
			// subnet can also determine the specified network
			specifiedNetwork = subnet.Spec.Network
		}
	}

	// Existing IP Instances Validation
	if len(specifiedNetwork) > 0 {
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

		for i := range ipList.Items {
			var ipInstance = &ipList.Items[i]
			// terminating ipInstance should be ignored
			if ipInstance.DeletionTimestamp == nil {
				switch {
				case ipInstance.Spec.Network != specifiedNetwork:
					return admission.Denied(fmt.Sprintf(
						"pod has assigned ip %s of network %s, cannot assign to another network %s",
						ipInstance.Spec.Address.IP,
						ipInstance.Spec.Network,
						specifiedNetwork,
					))
				case len(specifiedSubnet) > 0 && ipInstance.Spec.Subnet != specifiedSubnet:
					return admission.Denied(fmt.Sprintf(
						"pod has assigend ip %s of subnet %s, cannot assign to another subnet %s",
						ipInstance.Spec.Address.IP,
						ipInstance.Spec.Subnet,
						specifiedSubnet,
					))
				}
			}
		}
	}

	// IP Pool Validation
	var ipPool string
	if ipPool = pod.Annotations[constants.AnnotationIPPool]; len(ipPool) > 0 {
		if len(specifiedNetwork) == 0 {
			return admission.Denied("ip pool and network(subnet) must be specified at the same time")
		}
		ips := strings.Split(ipPool, ",")
		for _, ip := range ips {
			if utils.NormalizedIP(ip) != ip {
				return admission.Denied(fmt.Sprintf("ip pool has invalid ip %s", ip))
			}
		}
	}

	// Overlay network capacity validation
	if feature.DualStackEnabled() && networkType == ipamtypes.Overlay {
		networkList := &networkingv1.NetworkList{}
		if err = handler.Client.List(ctx, networkList); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		for i := range networkList.Items {
			network := &networkList.Items[i]
			if network.Spec.Type == networkingv1.NetworkTypeOverlay {
				switch ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily]) {
				case ipamtypes.IPv4Only:
					if network.Status.Statistics.Available <= 0 {
						return admission.Denied("lacking ipv4 addresses for overlay mode")
					}
				case ipamtypes.IPv6Only:
					if network.Status.IPv6Statistics.Available <= 0 {
						return admission.Denied("lacking ipv6 addresses for overlay mode")
					}
				case ipamtypes.DualStack:
					if network.Status.DualStackStatistics.Available <= 0 {
						return admission.Denied("lacking dual stack addresses for overlay mode")
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
