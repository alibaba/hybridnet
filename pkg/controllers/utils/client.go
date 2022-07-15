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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
)

func ListNetworks(ctx context.Context, client client.Reader, opts ...client.ListOption) (*networkingv1.NetworkList, error) {
	var networkList = networkingv1.NetworkList{}
	if err := client.List(ctx, &networkList, opts...); err != nil {
		return nil, err
	}
	return &networkList, nil
}

func GetNetwork(ctx context.Context, client client.Reader, name string) (*networkingv1.Network, error) {
	var network = networkingv1.Network{}
	if err := client.Get(ctx, types.NamespacedName{Name: name}, &network); err != nil {
		return nil, err
	}
	return &network, nil
}

func GetPod(ctx context.Context, client client.Reader, name, namespace string) (*corev1.Pod, error) {
	var pod = corev1.Pod{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

func ListSubnets(ctx context.Context, client client.Reader, opts ...client.ListOption) (*networkingv1.SubnetList, error) {
	var subnetList = networkingv1.SubnetList{}
	if err := client.List(ctx, &subnetList, opts...); err != nil {
		return nil, err
	}
	return &subnetList, nil
}

func GetSubnet(ctx context.Context, client client.Reader, name string) (*networkingv1.Subnet, error) {
	var subnet = networkingv1.Subnet{}
	if err := client.Get(ctx, types.NamespacedName{Name: name}, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

func ListIPInstances(ctx context.Context, client client.Reader, opts ...client.ListOption) (*networkingv1.IPInstanceList, error) {
	var ipList = networkingv1.IPInstanceList{}
	if err := client.List(ctx, &ipList, opts...); err != nil {
		return nil, err
	}
	return &ipList, nil
}

func ListActiveNodesToNames(ctx context.Context, client client.Reader, opts ...client.ListOption) ([]string, error) {
	var nodeList = corev1.NodeList{}
	if err := client.List(ctx, &nodeList, opts...); err != nil {
		// TODO: handle error here
		return nil, err
	}
	var names = make([]string, len(nodeList.Items))
	for i := range nodeList.Items {
		// node is active iff it is not in terminating
		if nodeList.Items[i].DeletionTimestamp == nil {
			names[i] = nodeList.Items[i].GetName()
		}
	}
	return names, nil
}

func ListActiveSubnetsToNames(ctx context.Context, client client.Reader, opts ...client.ListOption) ([]string, error) {
	subnetList, err := ListSubnets(ctx, client, opts...)
	if err != nil {
		return nil, err
	}

	var names = make([]string, len(subnetList.Items))
	for i := range subnetList.Items {
		// subnet is active iff it is not in terminating
		if subnetList.Items[i].DeletionTimestamp == nil {
			names[i] = subnetList.Items[i].GetName()
		}
	}
	return names, nil
}

func ListRemoteSubnets(ctx context.Context, client client.Reader, opts ...client.ListOption) (*multiclusterv1.RemoteSubnetList, error) {
	var remoteSubnetList = multiclusterv1.RemoteSubnetList{}
	if err := client.List(ctx, &remoteSubnetList, opts...); err != nil {
		return nil, err
	}
	return &remoteSubnetList, nil
}

func ListRemoteVteps(ctx context.Context, client client.Reader, opts ...client.ListOption) (*multiclusterv1.RemoteVtepList, error) {
	var remoteVtepList = multiclusterv1.RemoteVtepList{}
	if err := client.List(ctx, &remoteVtepList, opts...); err != nil {
		return nil, err
	}
	return &remoteVtepList, nil
}

func ListRemoteClusters(ctx context.Context, client client.Reader, opts ...client.ListOption) (*multiclusterv1.RemoteClusterList, error) {
	var remoteClusterList = multiclusterv1.RemoteClusterList{}
	if err := client.List(ctx, &remoteClusterList, opts...); err != nil {
		return nil, err
	}
	return &remoteClusterList, nil
}

func GetRemoteCluster(ctx context.Context, client client.Reader, name string) (*multiclusterv1.RemoteCluster, error) {
	var remoteCluster = &multiclusterv1.RemoteCluster{}
	if err := client.Get(ctx, types.NamespacedName{Name: name}, remoteCluster); err != nil {
		return nil, err
	}
	return remoteCluster, nil
}

func FindUnderlayNetworkForNodeName(ctx context.Context, client client.Reader, nodeName string) (underlayNetworkName string, err error) {
	var node = &corev1.Node{}
	if err = client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", err
	}

	return FindUnderlayNetworkForNode(ctx, client, node.GetLabels())
}

func FindUnderlayNetworkForNode(ctx context.Context, client client.Reader, nodeLabels map[string]string) (underlayNetworkName string, err error) {
	networkList, err := ListNetworks(ctx, client)
	if err != nil {
		return "", err
	}

	for i := range networkList.Items {
		var network = networkList.Items[i]
		// TODO: explicit network type
		if networkingv1.GetNetworkType(&network) == networkingv1.NetworkTypeUnderlay && len(network.Spec.NodeSelector) > 0 {
			if labels.SelectorFromSet(network.Spec.NodeSelector).Matches(labels.Set(nodeLabels)) {
				return network.Name, nil
			}
		}
	}
	return "", nil
}

func FindBGPUnderlayNetworkForNode(ctx context.Context, client client.Reader, nodeLabels map[string]string) (underlayNetworkName string, err error) {
	networkList, err := ListNetworks(ctx, client)
	if err != nil {
		return "", err
	}

	for i := range networkList.Items {
		var network = networkList.Items[i]
		// TODO: explicit network type
		if networkingv1.GetNetworkMode(&network) == networkingv1.NetworkModeBGP && len(network.Spec.NodeSelector) > 0 {
			if labels.SelectorFromSet(network.Spec.NodeSelector).Matches(labels.Set(nodeLabels)) {
				return network.Name, nil
			}
		}
	}
	return "", nil
}

func FindOverlayNetwork(ctx context.Context, client client.Reader) (overlayNetworkName string, err error) {
	var networkList *networkingv1.NetworkList
	if networkList, err = ListNetworks(ctx, client); err != nil {
		return
	}

	for i := range networkList.Items {
		var network = networkList.Items[i]
		if networkingv1.GetNetworkType(&network) == networkingv1.NetworkTypeOverlay {
			return network.Name, nil
		}
	}
	return
}

func FindGlobalBGPNetwork(ctx context.Context, client client.Reader) (globalBGPNetworkName string, err error) {
	var networkList *networkingv1.NetworkList
	if networkList, err = ListNetworks(ctx, client); err != nil {
		return
	}

	for i := range networkList.Items {
		var network = networkList.Items[i]
		if networkingv1.GetNetworkType(&network) == networkingv1.NetworkTypeGlobalBGP {
			return network.Name, nil
		}
	}
	return
}

func FindOverlayNetworkNetID(ctx context.Context, client client.Reader) (*int32, error) {
	networkList, err := ListNetworks(ctx, client)
	if err != nil {
		return nil, err
	}
	for i := range networkList.Items {
		var network = &networkList.Items[i]
		if networkingv1.GetNetworkType(network) == networkingv1.NetworkTypeOverlay {
			if network.Spec.NetID == nil {
				return nil, nil
			}
			netID := *network.Spec.NetID
			return &netID, nil
		}
	}
	return nil, fmt.Errorf("no overlay network found")
}

func DetectNetworkAttachmentOfNode(ctx context.Context, client client.Reader, node *corev1.Node) (underlayAttached, overlayAttached bool, err error) {
	var underlayNetworkName string
	if underlayNetworkName, err = FindUnderlayNetworkForNode(ctx, client, node.GetLabels()); err != nil {
		return
	}
	var overlayNetworkName string
	if overlayNetworkName, err = FindOverlayNetwork(ctx, client); err != nil {
		return
	}

	return underlayNetworkName != "", overlayNetworkName != "", nil
}

// ListAllocatedIPInstances will list allocated (non-terminating) IPInstances by some specified filters
func ListAllocatedIPInstances(ctx context.Context, c client.Reader, opts ...client.ListOption) (ips []*networkingv1.IPInstance, err error) {
	var ipList *networkingv1.IPInstanceList
	if ipList, err = ListIPInstances(ctx, c, opts...); err != nil {
		return
	}
	for i := range ipList.Items {
		var ip = &ipList.Items[i]
		// terminating ip should not be picked ip
		if ip.DeletionTimestamp == nil {
			ips = append(ips, ip.DeepCopy())
		}
	}
	return
}

func ListAllocatedIPInstancesOfPod(ctx context.Context, c client.Reader, pod *corev1.Pod) (ips []*networkingv1.IPInstance, err error) {
	return ListAllocatedIPInstances(ctx, c,
		client.MatchingLabels{
			constants.LabelPod: pod.Name,
		},
		client.InNamespace(pod.Namespace),
	)
}

func GetClusterUUID(ctx context.Context, c client.Reader) (types.UID, error) {
	var namespace = &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: "kube-system"}, namespace); err != nil {
		return "", err
	}
	return namespace.UID, nil
}
