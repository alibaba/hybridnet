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

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/feature"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var networkGVK = gvkConverter(ramav1.SchemeGroupVersion.WithKind("Network"))

func init() {
	createHandlers[networkGVK] = NetworkCreateValidation
	updateHandlers[networkGVK] = NetworkUpdateValidation
	deleteHandlers[networkGVK] = NetworkDeleteValidation
}

func NetworkCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	network := &ramav1.Network{}
	err := handler.Decoder.Decode(*req, network)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch ramav1.GetNetworkType(network) {
	case ramav1.NetworkTypeUnderlay:
		if network.Spec.NodeSelector == nil || len(network.Spec.NodeSelector) == 0 {
			return admission.Denied("must have node selector")
		}
	case ramav1.NetworkTypeOverlay:
		// check uniqueness
		networks := &ramav1.NetworkList{}
		if err = handler.Client.List(ctx, networks); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		for i := range networks.Items {
			if ramav1.GetNetworkType(&networks.Items[i]) == ramav1.NetworkTypeOverlay {
				return admission.Denied("must have one overlay network at most")
			}
		}

		// check node selector
		if network.Spec.NodeSelector != nil && len(network.Spec.NodeSelector) > 0 {
			return admission.Denied("must not assign node selector for overlay network")
		}

		// check net id
		if network.Spec.NetID == nil {
			return admission.Denied("must assign net ID for overlay network")
		}
	default:
		return admission.Denied(fmt.Sprintf("unknown network type %s", ramav1.GetNetworkType(network)))
	}

	return admission.Allowed("validation pass")
}

func NetworkUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	var err error
	oldN, newN := &ramav1.Network{}, &ramav1.Network{}
	if err = handler.Decoder.DecodeRaw(req.Object, newN); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err = handler.Decoder.DecodeRaw(req.OldObject, oldN); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if ramav1.GetNetworkType(oldN) != ramav1.GetNetworkType(newN) {
		return admission.Denied("network type must not be changed")
	}

	switch ramav1.GetNetworkType(newN) {
	case ramav1.NetworkTypeUnderlay:
	case ramav1.NetworkTypeOverlay:
		if newN.Spec.NodeSelector != nil && len(newN.Spec.NodeSelector) > 0 {
			return admission.Denied("node selector must not be assigned for overlay network")
		}
	default:
		return admission.Denied(fmt.Sprintf("unknown network type %s", ramav1.GetNetworkType(newN)))
	}

	if !reflect.DeepEqual(oldN.Spec.NetID, newN.Spec.NetID) {
		return admission.Denied("net ID must not be changed")
	}

	return admission.Allowed("validation pass")
}

func NetworkDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	var err error
	network := &ramav1.Network{}
	if err = handler.Client.Get(ctx, types.NamespacedName{Name: req.Name}, network); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	subnetList := &ramav1.SubnetList{}
	if err = handler.Client.List(ctx, subnetList); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	for _, subnet := range subnetList.Items {
		if subnet.Spec.Network == network.Name {
			return admission.Denied(fmt.Sprintf("still have child subnet %s", subnet.Name))
		}
	}

	if network.Spec.Type == ramav1.NetworkTypeOverlay && feature.MultiClusterEnabled() {
		remoteClusterLister := &ramav1.RemoteClusterList{}
		if err = handler.Client.List(ctx, remoteClusterLister); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if len(remoteClusterLister.Items) != 0 {
			return admission.Denied(fmt.Sprintf("still have remote cluster. number=%v", len(remoteClusterLister.Items)))
		}
	}

	return admission.Allowed("validation pass")
}
