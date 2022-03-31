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
)

func ListNetworks(client client.Reader, opts ...client.ListOption) (*networkingv1.NetworkList, error) {
	var networkList = networkingv1.NetworkList{}
	if err := client.List(context.TODO(), &networkList, opts...); err != nil {
		return nil, err
	}
	return &networkList, nil
}

func GetNetwork(client client.Reader, name string) (*networkingv1.Network, error) {
	var network = networkingv1.Network{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: name}, &network); err != nil {
		return nil, err
	}
	return &network, nil
}

func ListSubnets(client client.Reader, opts ...client.ListOption) (*networkingv1.SubnetList, error) {
	var subnetList = networkingv1.SubnetList{}
	if err := client.List(context.TODO(), &subnetList, opts...); err != nil {
		return nil, err
	}
	return &subnetList, nil
}

func GetSubnet(client client.Reader, name string) (*networkingv1.Subnet, error) {
	var subnet = networkingv1.Subnet{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: name}, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

func ListIPInstances(client client.Reader, opts ...client.ListOption) (*networkingv1.IPInstanceList, error) {
	var ipList = networkingv1.IPInstanceList{}
	if err := client.List(context.TODO(), &ipList, opts...); err != nil {
		return nil, err
	}
	return &ipList, nil
}

func ListNodesToNames(client client.Reader, opts ...client.ListOption) ([]string, error) {
	var nodeList = corev1.NodeList{}
	if err := client.List(context.TODO(), &nodeList, opts...); err != nil {
		// TODO: handle error here
		return nil, err
	}
	var names = make([]string, len(nodeList.Items))
	for i := range nodeList.Items {
		names[i] = nodeList.Items[i].GetName()
	}
	return names, nil
}

func ListSubnetsToNames(client client.Reader, opts ...client.ListOption) ([]string, error) {
	subnetList, err := ListSubnets(client, opts...)
	if err != nil {
		return nil, err
	}

	var names = make([]string, len(subnetList.Items))
	for i := range subnetList.Items {
		names[i] = subnetList.Items[i].GetName()
	}
	return names, nil
}

func ListRemoteSubnets(client client.Reader, opts ...client.ListOption) (*multiclusterv1.RemoteSubnetList, error) {
	var remoteSubnetList = multiclusterv1.RemoteSubnetList{}
	if err := client.List(context.TODO(), &remoteSubnetList, opts...); err != nil {
		return nil, err
	}
	return &remoteSubnetList, nil
}

func ListRemoteVteps(client client.Reader, opts ...client.ListOption) (*multiclusterv1.RemoteVtepList, error) {
	var remoteVtepList = multiclusterv1.RemoteVtepList{}
	if err := client.List(context.TODO(), &remoteVtepList, opts...); err != nil {
		return nil, err
	}
	return &remoteVtepList, nil
}

func ListRemoteClusters(client client.Reader, opts ...client.ListOption) (*multiclusterv1.RemoteClusterList, error) {
	var remoteClusterList = multiclusterv1.RemoteClusterList{}
	if err := client.List(context.TODO(), &remoteClusterList, opts...); err != nil {
		return nil, err
	}
	return &remoteClusterList, nil
}

func GetRemoteCluster(client client.Reader, name string) (*multiclusterv1.RemoteCluster, error) {
	var remoteCluster = &multiclusterv1.RemoteCluster{}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: name}, remoteCluster); err != nil {
		return nil, err
	}
	return remoteCluster, nil
}

func FindUnderlayNetworkForNodeName(client client.Reader, nodeName string) (underlayNetworkName string, err error) {
	var node = &corev1.Node{}
	if err = client.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", err
	}

	return FindUnderlayNetworkForNode(client, node.GetLabels())
}

func FindUnderlayNetworkForNode(client client.Reader, nodeLabels map[string]string) (underlayNetworkName string, err error) {
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

func FindOverlayNetwork(client client.Reader) (overlayNetworkName string, err error) {
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

func FindOverlayNetworkNetID(client client.Reader) (*int32, error) {
	networkList, err := ListNetworks(client)
	if err != nil {
		return nil, err
	}
	for i := range networkList.Items {
		var network = &networkList.Items[i]
		if network.Spec.Type == networkingv1.NetworkTypeOverlay {
			if network.Spec.NetID == nil {
				return nil, nil
			}
			netID := *network.Spec.NetID
			return &netID, nil
		}
	}
	return nil, fmt.Errorf("no overlay network found")
}

func DetectNetworkAttachmentOfNode(client client.Reader, node *corev1.Node) (underlayAttached, overlayAttached bool, err error) {
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

func ListAllocatedIPInstancesOfPod(c client.Reader, pod *corev1.Pod) (ips []*networkingv1.IPInstance, err error) {
	var ipList *networkingv1.IPInstanceList
	if ipList, err = ListIPInstances(c, client.InNamespace(pod.Namespace)); err != nil {
		return
	}
	for i := range ipList.Items {
		var ip = &ipList.Items[i]
		// terminating ip should not be picked ip
		if networkingv1.GetPodOfIPInstance(ip) == pod.Name && ip.DeletionTimestamp == nil {
			ips = append(ips, ip.DeepCopy())
		}
	}
	return
}

func GetIPOfPod(c client.Reader, pod *corev1.Pod) (string, error) {
	ipList, err := ListIPInstances(c, client.InNamespace(pod.Namespace))
	if err != nil {
		return "", err
	}

	for i := range ipList.Items {
		var ip = &ipList.Items[i]
		// terminating ip should not be picked ip
		if networkingv1.GetPodOfIPInstance(ip) == pod.Name && ip.DeletionTimestamp == nil {
			return ToIPFormat(ip.Name), nil
		}
	}
	return "", nil
}

func ListIPsOfPod(c client.Reader, pod *corev1.Pod) ([]string, error) {
	ipList, err := ListIPInstances(c, client.InNamespace(pod.Namespace))
	if err != nil {
		return nil, err
	}

	var v4, v6 []string
	for i := range ipList.Items {
		var ip = &ipList.Items[i]
		// terminating ip should not be picked ip
		if networkingv1.GetPodOfIPInstance(ip) == pod.Name && ip.DeletionTimestamp == nil {
			ipStr, isIPv6 := ToIPFormatWithFamily(ip.Name)
			if isIPv6 {
				v6 = append(v6, ipStr)
			} else {
				v4 = append(v4, ipStr)
			}
		}
	}
	return append(v4, v6...), nil
}

func GetClusterUUID(c client.Reader) (types.UID, error) {
	var namespace = &corev1.Namespace{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: "kube-system"}, namespace); err != nil {
		return "", err
	}
	return namespace.UID, nil
}
