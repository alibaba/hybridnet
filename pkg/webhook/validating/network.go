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
	"net"
	"net/http"
	"reflect"

	"github.com/alibaba/hybridnet/pkg/constants"

	"sigs.k8s.io/controller-runtime/pkg/client"

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

	networkType := networkingv1.GetNetworkType(network)

	switch networkType {
	case networkingv1.NetworkTypeUnderlay:
		if network.Spec.NodeSelector == nil || len(network.Spec.NodeSelector) == 0 {
			return webhookutils.AdmissionDeniedWithLog("must have node selector for underlay network", logger)
		}
	case networkingv1.NetworkTypeOverlay:
		// check uniqueness
		if exist, _, err := checkNetworkTypeExist(ctx, handler.Client, networkType); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		} else if exist {
			return webhookutils.AdmissionDeniedWithLog("must have one overlay network at most", logger)
		}

		// check node selector
		if network.Spec.NodeSelector != nil && len(network.Spec.NodeSelector) > 0 {
			return webhookutils.AdmissionDeniedWithLog("must not assign node selector for overlay network", logger)
		}

		// check net id
		if network.Spec.NetID == nil {
			return webhookutils.AdmissionDeniedWithLog("must assign net ID for overlay network", logger)
		}
	case networkingv1.NetworkTypeGlobalBGP:
		// check uniqueness
		if exist, _, err := checkNetworkTypeExist(ctx, handler.Client, networkType); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		} else if exist {
			return webhookutils.AdmissionDeniedWithLog("must have one global bgp network at most", logger)
		}

		// check node selector
		if network.Spec.NodeSelector != nil && len(network.Spec.NodeSelector) > 0 {
			return webhookutils.AdmissionDeniedWithLog("must not assign node selector for global bgp network", logger)
		}

		// check net id
		if network.Spec.NetID != nil {
			return webhookutils.AdmissionDeniedWithLog("must not assign net ID for global bgp network", logger)
		}
	default:
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("unknown network type %s", networkingv1.GetNetworkType(network)), logger)
	}

	switch networkingv1.GetNetworkMode(network) {
	case networkingv1.NetworkModeBGP:
		if networkType != networkingv1.NetworkTypeUnderlay {
			return admission.Denied("BGP mode can only be used for underlay network")
		}

		// check net id
		if network.Spec.NetID == nil {
			return admission.Denied("must assign net ID for bgp network")
		}

		if network.Spec.Config == nil || len(network.Spec.Config.BGPPeers) == 0 {
			return admission.Denied("at least one bgp router need to be set")
		}

		for _, peer := range network.Spec.Config.BGPPeers {
			if net.ParseIP(peer.Address) == nil {
				return admission.Denied(fmt.Sprintf("invalid bgp peer ip address %v", peer.Address))
			}

			if peer.ASN == 0 {
				return admission.Denied(fmt.Sprintf("bgp peer %v's AS number need to be set", peer.Address))
			}
		}
	case networkingv1.NetworkModeVlan:
		if networkType != networkingv1.NetworkTypeUnderlay {
			return admission.Denied("VLAN mode can only be used for underlay network")
		}
	case networkingv1.NetworkModeVxlan:
		if networkType != networkingv1.NetworkTypeOverlay {
			return admission.Denied("VXLAN mode can only be used for overlay network")
		}
	case networkingv1.NetworkModeGlobalBGP:
		if networkType != networkingv1.NetworkTypeGlobalBGP {
			return admission.Denied("GlobalBGP mode can only be used for global bgp network")
		}
	default:
		return admission.Denied(fmt.Sprintf("unknown network mode %s", networkingv1.GetNetworkMode(network)))
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
		if newN.Spec.NodeSelector == nil || len(newN.Spec.NodeSelector) == 0 {
			return admission.Denied("must have node selector for underlay network")
		}
	case networkingv1.NetworkTypeOverlay:
		if newN.Spec.NodeSelector != nil && len(newN.Spec.NodeSelector) > 0 {
			return webhookutils.AdmissionDeniedWithLog("node selector must not be assigned for overlay network", logger)
		}
	case networkingv1.NetworkTypeGlobalBGP:
		if newN.Spec.NodeSelector != nil && len(newN.Spec.NodeSelector) > 0 {
			return webhookutils.AdmissionDeniedWithLog("node selector must not be assigned for global bgp network", logger)
		}
	default:
		return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("unknown network type %s", networkingv1.GetNetworkType(newN)), logger)
	}

	if oldN.Spec.Mode != newN.Spec.Mode {
		return admission.Denied("network mode must not be changed")
	}

	switch networkingv1.GetNetworkMode(newN) {
	case networkingv1.NetworkModeBGP:
		if len(newN.Spec.Config.BGPPeers) == 0 {
			return admission.Denied("at least one bgp router need to be set")
		}

		for _, peer := range newN.Spec.Config.BGPPeers {
			if net.ParseIP(peer.Address) == nil {
				return admission.Denied(fmt.Sprintf("invalid bgp peer ip address %v", peer.Address))
			}
		}
	case networkingv1.NetworkModeVlan, networkingv1.NetworkModeVxlan, networkingv1.NetworkModeGlobalBGP:
	default:
		return admission.Denied(fmt.Sprintf("unknown network mode %s", networkingv1.GetNetworkMode(newN)))
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

	if networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay && feature.MultiClusterEnabled() {
		remoteClusterList := &multiclusterv1.RemoteClusterList{}
		if err = handler.Client.List(ctx, remoteClusterList); err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		}
		if len(remoteClusterList.Items) != 0 {
			return webhookutils.AdmissionDeniedWithLog(fmt.Sprintf("still have remote cluster. number=%v", len(remoteClusterList.Items)), logger)
		}
	}

	switch networkingv1.GetNetworkMode(network) {
	case networkingv1.NetworkModeBGP:
		globalBGPExist, globalBGPNetwork, err := checkNetworkTypeExist(ctx, handler.Client, networkingv1.NetworkTypeGlobalBGP)
		if err != nil {
			return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
		} else if !globalBGPExist {
			break
		}

		if len(network.Status.NodeList) != 0 {
			for _, node := range network.Status.NodeList {
				ipInstanceList := &networkingv1.IPInstanceList{}

				if err := handler.Client.List(context.TODO(), ipInstanceList, client.MatchingLabels{
					constants.LabelNode:    node,
					constants.LabelNetwork: globalBGPNetwork,
				}); err != nil {
					return webhookutils.AdmissionErroredWithLog(http.StatusInternalServerError, err, logger)
				}

				if len(ipInstanceList.Items) != 0 {
					return webhookutils.AdmissionDeniedWithLog("still have global bgp ip instance in use", logger)
				}
			}
		}
	}

	return admission.Allowed("validation pass")
}

func checkNetworkTypeExist(ctx context.Context, client client.Reader, networkType networkingv1.NetworkType) (bool, string, error) {
	networks := &networkingv1.NetworkList{}
	if err := client.List(ctx, networks); err != nil {
		return false, "", err
	}
	for i := range networks.Items {
		if networkingv1.GetNetworkType(&networks.Items[i]) == networkType {
			return true, networks.Items[i].Name, nil
		}
	}
	return false, "", nil
}
