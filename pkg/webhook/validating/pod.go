/*
  Copyright 2021 The Rama Authors.

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

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/feature"
	ipamtypes "github.com/oecp/rama/pkg/ipam/types"
	"github.com/oecp/rama/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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
		specifiedNetwork = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedNetwork], pod.Labels[constants.LabelSpecifiedNetwork])
		networkType      = ipamtypes.ParseNetworkTypeFromString(utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType]))
	)
	if len(specifiedNetwork) > 0 {
		network := &ramav1.Network{}
		if err = handler.Cache.Get(ctx, types.NamespacedName{Name: specifiedNetwork}, network); err != nil {
			if errors.IsNotFound(err) {
				return admission.Denied(fmt.Sprintf("specified network %s not found", specifiedNetwork))
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// check network type
		if !stringEqualCaseInsensitive(string(ramav1.GetNetworkType(network)), string(networkType)) {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("specified network type mismatch %s %s", networkType, ramav1.GetNetworkType(network)))

		}

		ipList := &ramav1.IPInstanceList{}
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
			if ipInstance.DeletionTimestamp == nil && ipInstance.Spec.Network != specifiedNetwork {
				return admission.Denied(fmt.Sprintf(
					"pod has assigned ip %s of network %s, cannot assign to another network %s",
					ipInstance.Spec.Address.IP,
					ipInstance.Spec.Network,
					specifiedNetwork,
				))
			}
		}
	}

	// Specified Subnet Validation
	var specifiedSubnet string
	if specifiedSubnet = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet], pod.Labels[constants.LabelSpecifiedSubnet]); len(specifiedSubnet) > 0 {
		if len(specifiedNetwork) == 0 {
			return admission.Denied("subnet and network must be specified at the same time")
		}
		subnet := &ramav1.Subnet{}
		if err = handler.Cache.Get(ctx, types.NamespacedName{Name: specifiedSubnet}, subnet); err != nil {
			if errors.IsNotFound(err) {
				return admission.Denied(fmt.Sprintf("specified subnet %s not found", specifiedSubnet))
			}
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if subnet.Spec.Network != specifiedNetwork {
			return admission.Denied(fmt.Sprintf("specified subnet %s not belong to specified network %s",
				specifiedSubnet,
				specifiedNetwork,
			))
		}
	}

	// IP Pool Validation
	var ipPool string
	if ipPool = pod.Annotations[constants.AnnotationIPPool]; len(ipPool) > 0 {
		if len(specifiedNetwork) == 0 {
			return admission.Denied("ip pool and network must be specified at the same time")
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
		networkList := &ramav1.NetworkList{}
		if err = handler.Client.List(ctx, networkList); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		for i := range networkList.Items {
			network := &networkList.Items[i]
			if network.Spec.Type == ramav1.NetworkTypeOverlay {
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
