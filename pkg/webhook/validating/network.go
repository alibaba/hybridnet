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

	webhookutils "github.com/alibaba/hybridnet/pkg/webhook/utils"
	"sigs.k8s.io/controller-runtime/pkg/log"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/feature"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var networkGVK = gvkConverter(networkingv1.GroupVersion.WithKind("Network"))

func init() {
	createHandlers[networkGVK] = NetworkCreateValidation
	updateHandlers[networkGVK] = NetworkUpdateValidation
	deleteHandlers[networkGVK] = NetworkDeleteValidation
}

func NetworkCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	network := &networkingv1.Network{}
	err := handler.Decoder.Decode(*req, network)
	if err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	switch networkingv1.GetNetworkType(network) {
	case networkingv1.NetworkTypeUnderlay:
		if network.Spec.NodeSelector == nil || len(network.Spec.NodeSelector) == 0 {
			return webhookutils.AdmissionDeniedWithLog("must have node selector", logger)
		}
	case networkingv1.NetworkTypeOverlay:
		// check uniqueness
		networks := &networkingv1.NetworkList{}
		if err = handler.Client.List(ctx, networks); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}
		for i := range networks.Items {
			if networkingv1.GetNetworkType(&networks.Items[i]) == networkingv1.NetworkTypeOverlay {
				return webhookutils.AdmissionDeniedWithLog("must have one overlay network at most", logger)
			}
		}

		// check node selector
		if network.Spec.NodeSelector != nil && len(network.Spec.NodeSelector) > 0 {
			return webhookutils.AdmissionDeniedWithLog("must not assign node selector for overlay network", logger)
		}

		// check net id
		if network.Spec.NetID == nil {
			return webhookutils.AdmissionDeniedWithLog("must assign net ID for overlay network", logger)
		}
	default:
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("unknown network type %s", networkingv1.GetNetworkType(network)), logger)
	}

	return admission.Allowed("validation pass")
}

func NetworkUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	var err error
	oldN, newN := &networkingv1.Network{}, &networkingv1.Network{}
	if err = handler.Decoder.DecodeRaw(req.Object, newN); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}
	if err = handler.Decoder.DecodeRaw(req.OldObject, oldN); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	if networkingv1.GetNetworkType(oldN) != networkingv1.GetNetworkType(newN) {
		return webhookutils.AdmissionDeniedWithLog("network type must not be changed", logger)
	}

	switch networkingv1.GetNetworkType(newN) {
	case networkingv1.NetworkTypeUnderlay:
	case networkingv1.NetworkTypeOverlay:
		if newN.Spec.NodeSelector != nil && len(newN.Spec.NodeSelector) > 0 {
			return webhookutils.AdmissionDeniedWithLog("node selector must not be assigned for overlay network", logger)
		}
	default:
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("unknown network type %s", networkingv1.GetNetworkType(newN)), logger)
	}

	if !reflect.DeepEqual(oldN.Spec.NetID, newN.Spec.NetID) {
		return webhookutils.AdmissionDeniedWithLog("net ID must not be changed", logger)
	}

	return admission.Allowed("validation pass")
}

func NetworkDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	logger := log.FromContext(ctx)

	var err error
	network := &networkingv1.Network{}
	if err = handler.Client.Get(ctx, types.NamespacedName{Name: req.Name}, network); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusBadRequest, err, logger)
	}

	subnetList := &networkingv1.SubnetList{}
	if err = handler.Client.List(ctx, subnetList); err != nil {
		return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
	}

	for _, subnet := range subnetList.Items {
		if subnet.Spec.Network == network.Name {
			return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("still have child subnet %s", subnet.Name), logger)
		}
	}

	if network.Spec.Type == networkingv1.NetworkTypeOverlay && feature.MultiClusterEnabled() {
		remoteClusterList := &multiclusterv1.RemoteClusterList{}
		if err = handler.Client.List(ctx, remoteClusterList); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}
		if len(remoteClusterList.Items) != 0 {
			return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("still have remote cluster. number=%v", len(remoteClusterList.Items)), logger)
		}
	}

	return admission.Allowed("validation pass")
}
