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
	"reflect"
	"strings"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	"github.com/oecp/rama/pkg/feature"
	"github.com/oecp/rama/pkg/utils"
	"github.com/oecp/rama/pkg/utils/transform"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	MaxSubnetCapacity = 1 << 16
)

var subnetGVK = gvkConverter(ramav1.SchemeGroupVersion.WithKind("Subnet"))

func init() {
	createHandlers[subnetGVK] = SubnetCreateValidation
	updateHandlers[subnetGVK] = SubnetUpdateValidation
	deleteHandlers[subnetGVK] = SubnetDeleteValidation
}

func SubnetCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	subnet := &ramav1.Subnet{}
	err := handler.Decoder.Decode(*req, subnet)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Parent Network validation
	if len(subnet.Spec.Network) == 0 {
		return admission.Denied("must have parent network")
	}

	network := &ramav1.Network{}
	err = handler.Client.Get(ctx, types.NamespacedName{Name: subnet.Spec.Network}, network)
	if err != nil {
		if errors.IsNotFound(err) {
			return admission.Denied(fmt.Sprintf("parent network %s does not exist", subnet.Spec.Network))
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// NetID validation
	switch ramav1.GetNetworkType(network) {
	case ramav1.NetworkTypeUnderlay:
		if subnet.Spec.NetID == nil {
			if network.Spec.NetID == nil {
				return admission.Denied("must have valid Net ID")
			}
		} else {
			if network.Spec.NetID != nil && *subnet.Spec.NetID != *network.Spec.NetID {
				return admission.Denied("have inconsistent Net ID with network")
			}
		}

		if subnet.Spec.Config != nil && subnet.Spec.Config.AutoNatOutgoing != nil {
			return admission.Denied("must not set autoNatOutgoing with underlay subnet")
		}

	case ramav1.NetworkTypeOverlay:
		if subnet.Spec.NetID != nil {
			return admission.Denied("must not assign net ID for overlay subnet")
		}
	}

	// Address Range validation
	if err = ramav1.ValidateAddressRange(&subnet.Spec.Range); err != nil {
		return admission.Denied(err.Error())
	}

	// Capacity validation
	if capacity := ramav1.CalculateCapacity(&subnet.Spec.Range); capacity > MaxSubnetCapacity {
		return admission.Denied(fmt.Sprintf("subnet contains more than %d IPs", MaxSubnetCapacity))
	}

	// Subnet overlap validation
	ipamSubnet := transform.TransferSubnetForIPAM(subnet)
	if err = ipamSubnet.Canonicalize(); err != nil {
		return admission.Denied(fmt.Sprintf("canonicalize subnet failed: %v", err))
	}
	subnetList := &ramav1.SubnetList{}
	if err = handler.Client.List(ctx, subnetList); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	for i := range subnetList.Items {
		comparedSubnet := transform.TransferSubnetForIPAM(&subnetList.Items[i])
		// we assume that all existing subnets all have been canonicalized
		if err = comparedSubnet.Canonicalize(); err == nil && comparedSubnet.Overlap(ipamSubnet) {
			return admission.Denied(fmt.Sprintf("overlap with existing subnet %s", comparedSubnet.Name))
		}
	}

	if feature.MultiClusterEnabled() {
		rcSubnetList := &ramav1.RemoteSubnetList{}
		if err = handler.Client.List(ctx, rcSubnetList); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		for _, rcSubnet := range rcSubnetList.Items {
			if utils.Intersect(subnet.Spec.Range.CIDR, subnet.Spec.Range.Version, rcSubnet.Spec.Range.CIDR, rcSubnet.Spec.Range.Version) {
				return admission.Denied(fmt.Sprintf("overlap with existing RemoteSubnet %s", rcSubnet.Name))
			}
		}
	}

	return admission.Allowed("validation pass")
}

func SubnetUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	var err error
	oldS, newS := &ramav1.Subnet{}, &ramav1.Subnet{}
	if err = handler.Decoder.DecodeRaw(req.Object, newS); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err = handler.Decoder.DecodeRaw(req.OldObject, oldS); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Parent Network validation
	if oldS.Spec.Network != newS.Spec.Network {
		return admission.Denied("must not change parent network")
	}

	network := &ramav1.Network{}
	err = handler.Client.Get(ctx, types.NamespacedName{Name: newS.Spec.Network}, network)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// NetID validation
	switch ramav1.GetNetworkType(network) {
	case ramav1.NetworkTypeUnderlay:
		if !reflect.DeepEqual(oldS.Spec.NetID, newS.Spec.NetID) {
			return admission.Denied("must not change net ID")
		}

		if newS.Spec.Config != nil && newS.Spec.Config.AutoNatOutgoing != nil {
			return admission.Denied("must not set autoNatOutgoing with underlay subnet")
		}

	case ramav1.NetworkTypeOverlay:
		if newS.Spec.NetID != nil {
			return admission.Denied("must not assign net ID for overlay subnet")
		}
	}

	// Address Range validation
	err = ramav1.ValidateAddressRange(&newS.Spec.Range)
	if err != nil {
		return admission.Denied(err.Error())
	}
	if oldS.Spec.Range.Start != newS.Spec.Range.Start {
		return admission.Denied("must not change range start")
	}
	if oldS.Spec.Range.End != newS.Spec.Range.End {
		return admission.Denied("must not change range end")
	}
	if oldS.Spec.Range.Gateway != newS.Spec.Range.Gateway {
		return admission.Denied("must not change range gateway")
	}
	if oldS.Spec.Range.CIDR != newS.Spec.Range.CIDR {
		return admission.Denied("must not change range CIDR")
	}

	return admission.Allowed("validation pass")
}

func SubnetDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	var err error
	subnet := &ramav1.Subnet{}
	if err = handler.Client.Get(ctx, types.NamespacedName{Name: req.Name}, subnet); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	ipList := &ramav1.IPInstanceList{}
	if err = handler.Client.List(ctx, ipList, client.MatchingLabels{constants.LabelSubnet: subnet.Name}); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if len(ipList.Items) > 0 {
		var usingIPs []string
		for _, ip := range ipList.Items {
			usingIPs = append(usingIPs, strings.Split(ip.Spec.Address.IP, "/")[0])
		}
		return admission.Denied(fmt.Sprintf("still have using ips %v", usingIPs))
	}

	return admission.Allowed("validation pass")
}
