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
	"reflect"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"

	webhookutils "github.com/alibaba/hybridnet/pkg/webhook/utils"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/utils"
	"github.com/alibaba/hybridnet/pkg/utils/transform"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	MaxSubnetCapacity = 1 << 16
)

var subnetGVK = gvkConverter(networkingv1.GroupVersion.WithKind("Subnet"))

func init() {
	createHandlers[subnetGVK] = SubnetCreateValidation
	updateHandlers[subnetGVK] = SubnetUpdateValidation
	deleteHandlers[subnetGVK] = SubnetDeleteValidation
}

func SubnetCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	subnet := &networkingv1.Subnet{}
	err := handler.Decoder.Decode(*req, subnet)
	if err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	// Parent Network validation
	if len(subnet.Spec.Network) == 0 {
		return webhookutils.AdmissionDeniedWithLog("must have parent network", logger)
	}

	network := &networkingv1.Network{}
	err = handler.Client.Get(ctx, types.NamespacedName{Name: subnet.Spec.Network}, network)
	if err != nil {
		if errors.IsNotFound(err) {
			return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("parent network %s does not exist", subnet.Spec.Network), logger)
		}
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
	}

	// NetID validation
	switch networkingv1.GetNetworkMode(network) {
	case networkingv1.NetworkModeVlan:
		if subnet.Spec.NetID == nil {
			if network.Spec.NetID == nil {
				return webhookutils.AdmissionDeniedWithLog("must have valid Net ID", logger)
			}
		} else {
			if network.Spec.NetID != nil && *subnet.Spec.NetID != *network.Spec.NetID {
				return webhookutils.AdmissionDeniedWithLog("have inconsistent Net ID with network", logger)
			}
		}

		if subnet.Spec.Config != nil && subnet.Spec.Config.AutoNatOutgoing != nil {
			return webhookutils.AdmissionDeniedWithLog("must not set autoNatOutgoing with underlay subnet", logger)
		}

		if len(subnet.Spec.Range.Gateway) == 0 {
			return admission.Denied("must assign gateway for a vlan subnet")
		}
	case networkingv1.NetworkModeBGP, networkingv1.NetworkModeGlobalBGP:
		if subnet.Spec.NetID != nil {
			return admission.Denied("must not assign net ID for (global) bgp subnet")
		}

		if subnet.Spec.Config != nil && subnet.Spec.Config.AutoNatOutgoing != nil {
			return admission.Denied("must not set autoNatOutgoing with underlay subnet")
		}
	case networkingv1.NetworkModeVxlan:
		if subnet.Spec.NetID != nil {
			return webhookutils.AdmissionDeniedWithLog("must not assign net ID for overlay subnet", logger)
		}
	}

	// Address Range validation
	if err = networkingv1.ValidateAddressRange(&subnet.Spec.Range); err != nil {
		return webhookutils.AdmissionDeniedWithLog(err.Error(), logger)
	}

	// IP Family validation
	if !feature.DualStackEnabled() && networkingv1.IsIPv6Subnet(subnet) {
		return webhookutils.AdmissionDeniedWithLog("ipv6 subnet non-supported if dualstack not enabled", logger)
	}

	// Capacity validation
	if capacity := networkingv1.CalculateCapacity(&subnet.Spec.Range); capacity > MaxSubnetCapacity {
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("subnet contains more than %d IPs", MaxSubnetCapacity), logger)
	}

	// Subnet overlap validation
	ipamSubnet := transform.TransferSubnetForIPAM(subnet)
	if err = ipamSubnet.Canonicalize(); err != nil {
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("canonicalize subnet failed: %v", err), logger)
	}
	subnetList := &networkingv1.SubnetList{}
	if err = handler.Client.List(ctx, subnetList); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
	}
	for i := range subnetList.Items {
		comparedSubnet := transform.TransferSubnetForIPAM(&subnetList.Items[i])
		// we assume that all existing subnets all have been canonicalized
		if err = comparedSubnet.Canonicalize(); err == nil && comparedSubnet.Overlap(ipamSubnet) {
			return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("overlap with existing subnet %s", comparedSubnet.Name), logger)
		}
	}

	if feature.MultiClusterEnabled() {
		rcSubnetList := &multiclusterv1.RemoteSubnetList{}
		if err = handler.Client.List(ctx, rcSubnetList); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}
		for _, rcSubnet := range rcSubnetList.Items {
			if utils.Intersect(&subnet.Spec.Range, &rcSubnet.Spec.Range) {
				return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("overlap with existing RemoteSubnet %s", rcSubnet.Name), logger)
			}
		}
	}

	return admission.Allowed("validation pass")
}

func SubnetUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	var err error
	oldS, newS := &networkingv1.Subnet{}, &networkingv1.Subnet{}
	if err = handler.Decoder.DecodeRaw(req.Object, newS); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}
	if err = handler.Decoder.DecodeRaw(req.OldObject, oldS); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	// Parent Network validation
	if oldS.Spec.Network != newS.Spec.Network {
		return webhookutils.AdmissionDeniedWithLog("must not change parent network", logger)
	}

	network := &networkingv1.Network{}
	err = handler.Client.Get(ctx, types.NamespacedName{Name: newS.Spec.Network}, network)
	if err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
	}

	// NetID validation
	switch networkingv1.GetNetworkMode(network) {
	case networkingv1.NetworkModeVlan:
		if !reflect.DeepEqual(oldS.Spec.NetID, newS.Spec.NetID) {
			return webhookutils.AdmissionDeniedWithLog("must not change net ID", logger)
		}

		if newS.Spec.Config != nil && newS.Spec.Config.AutoNatOutgoing != nil {
			return webhookutils.AdmissionDeniedWithLog("must not set autoNatOutgoing with vlan subnet", logger)
		}

	case networkingv1.NetworkModeVxlan:
		if newS.Spec.NetID != nil {
			return webhookutils.AdmissionDeniedWithLog("must not assign net ID for overlay subnet", logger)
		}
	case networkingv1.NetworkModeBGP, networkingv1.NetworkModeGlobalBGP:
		if newS.Spec.NetID != nil {
			return webhookutils.AdmissionDeniedWithLog("must not assign net ID for (global) bgp subnet", logger)
		}

		if newS.Spec.Config != nil && newS.Spec.Config.AutoNatOutgoing != nil {
			return webhookutils.AdmissionDeniedWithLog("must not set autoNatOutgoing with (global) bgp subnet", logger)
		}
	}

	// Address Range validation
	err = networkingv1.ValidateAddressRange(&newS.Spec.Range)
	if err != nil {
		return webhookutils.AdmissionDeniedWithLog(err.Error(), logger)
	}
	if oldS.Spec.Range.Start != newS.Spec.Range.Start {
		return webhookutils.AdmissionDeniedWithLog("must not change range start", logger)
	}
	if oldS.Spec.Range.End != newS.Spec.Range.End {
		return webhookutils.AdmissionDeniedWithLog("must not change range end", logger)
	}
	if oldS.Spec.Range.Gateway != newS.Spec.Range.Gateway {
		return webhookutils.AdmissionDeniedWithLog("must not change range gateway", logger)
	}
	if oldS.Spec.Range.CIDR != newS.Spec.Range.CIDR {
		return webhookutils.AdmissionDeniedWithLog("must not change range CIDR", logger)
	}
	if !utils.DeepEqualStringSlice(oldS.Spec.Range.ExcludeIPs, newS.Spec.Range.ExcludeIPs) {
		return webhookutils.AdmissionDeniedWithLog("must not change excluded IPs", logger)
	}

	return admission.Allowed("validation pass")
}

func SubnetDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	var err error
	subnet := &networkingv1.Subnet{}
	if err = handler.Client.Get(ctx, types.NamespacedName{Name: req.Name}, subnet); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	ipList := &networkingv1.IPInstanceList{}
	if err = handler.Client.List(ctx, ipList, client.MatchingLabels{constants.LabelSubnet: subnet.Name}); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
	}

	if len(ipList.Items) > 0 {
		var usingIPs []string
		for _, ip := range ipList.Items {
			usingIPs = append(usingIPs, strings.Split(ip.Spec.Address.IP, "/")[0])
		}
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("still have using ips %v", usingIPs), logger)
	}

	return admission.Allowed("validation pass")
}
