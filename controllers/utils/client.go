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

package utils

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingv1 "github.com/alibaba/hybridnet/apis/networking/v1"
)

func ListNetworks(client client.Client, opts ...client.ListOption) (*networkingv1.NetworkList, error) {
	var networkList = networkingv1.NetworkList{}
	if err := client.List(context.TODO(), &networkList, opts...); err != nil {
		return nil, err
	}
	return &networkList, nil
}

func GetNetwork(client client.Client, name string) (*networkingv1.Network, error) {
	var network = networkingv1.Network{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: name}, &network); err != nil {
		return nil, err
	}
	return &network, nil
}

func ListSubnets(client client.Client, opts ...client.ListOption) (*networkingv1.SubnetList, error) {
	var subnetList = networkingv1.SubnetList{}
	if err := client.List(context.TODO(), &subnetList, opts...); err != nil {
		return nil, err
	}
	return &subnetList, nil
}

func GetSubnet(client client.Client, name string) (*networkingv1.Subnet, error) {
	var subnet = networkingv1.Subnet{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: name}, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

func ListIPInstances(client client.Client, opts ...client.ListOption) (*networkingv1.IPInstanceList, error) {
	var ipList = networkingv1.IPInstanceList{}
	if err := client.List(context.TODO(), &ipList, opts...); err != nil {
		return nil, err
	}
	return &ipList, nil
}

func ListNodesToReconcileRequests(client client.Client) []reconcile.Request {
	var nodeList = corev1.NodeList{}
	if err := client.List(context.TODO(), &nodeList); err != nil {
		// TODO: handle error here
		return nil
	}
	var requests = make([]reconcile.Request, len(nodeList.Items))
	for i := range nodeList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: nodeList.Items[i].Name,
			},
		}
	}
	return requests
}

func FindUnderlayNetworkForNode(client client.Client, nodeLabels map[string]string) (underlayNetworkName string, err error) {
	networkList, err := ListNetworks(client)
	if err != nil {
		return "", err
	}

	for i := range networkList.Items {
		var network = networkList.Items[i]
		// TODO: explicit network type
		if network.Spec.Type != networkingv1.NetworkTypeOverlay && len(network.Spec.NodeSelector) > 0 {
			if labels.SelectorFromSet(network.Spec.NodeSelector).Matches(labels.Set(nodeLabels)) {
				return network.Name, nil
			}
		}
	}
	return "", nil
}

func FindOverlayNetwork(client client.Client) (overlayNetworkName string, err error) {
	var networkList *networkingv1.NetworkList
	if networkList, err = ListNetworks(client); err != nil {
		return
	}

	for i := range networkList.Items {
		var network = networkList.Items[i]
		// TODO: explicit network type
		if network.Spec.Type == networkingv1.NetworkTypeOverlay {
			return network.Name, nil
		}
	}
	return
}

func DetectNetworkAttachmentOfNode(client client.Client, node *corev1.Node) (underlayAttached, overlayAttached bool, err error) {
	var underlayNetworkName string
	if underlayNetworkName, err = FindUnderlayNetworkForNode(client, node.GetLabels()); err != nil {
		return
	}
	var overlayNetworkName string
	if overlayNetworkName, err = FindOverlayNetwork(client); err != nil {
		return
	}

	return underlayNetworkName != "", overlayNetworkName != "", nil
}
