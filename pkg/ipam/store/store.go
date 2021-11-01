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

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/mac"
)

type Worker struct {
	kubeClient      kubernetes.Interface
	hybridnetClient versioned.Interface
}

func NewWorker(kubeClient kubernetes.Interface, hybridnetClient versioned.Interface) *Worker {
	return &Worker{
		kubeClient:      kubeClient,
		hybridnetClient: hybridnetClient,
	}
}

func (w *Worker) Couple(pod *v1.Pod, ip *ipamtypes.IP) (err error) {
	var ipInstance *networkingv1.IPInstance

	ipInstance, err = w.createIP(pod, ip)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = w.deleteIP(ipInstance.Namespace, ipInstance.Name)
		}
	}()

	if err = w.updateIPStatus(ipInstance, pod.Spec.NodeName, pod.Name, pod.Namespace, string(networkingv1.IPPhaseUsing)); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = w.releaseIPFromPod(pod)
		}
	}()

	return w.patchIPtoPod(pod, ip)
}

func (w *Worker) ReCouple(pod *v1.Pod, ip *ipamtypes.IP) (err error) {
	var ipInstance *networkingv1.IPInstance

	ipInstance, err = w.getIP(pod.Namespace, ip)
	if err != nil {
		if errors.IsNotFound(err) {
			return w.Couple(pod, ip)
		}
		return
	}

	if err = w.patchIPLabels(ipInstance, pod.Name, pod.Spec.NodeName); err != nil {
		return err
	}

	if err = w.updateIPStatus(ipInstance, pod.Spec.NodeName, pod.Name, pod.Namespace, string(networkingv1.IPPhaseUsing)); err != nil {
		return err
	}

	return w.patchIPtoPod(pod, ip)
}

func (w *Worker) DeCouple(pod *v1.Pod) (err error) {
	if len(pod.Annotations[constants.AnnotationIP]) == 0 {
		return
	}

	var ipInstanceList *networkingv1.IPInstanceList
	if ipInstanceList, err = w.hybridnetClient.NetworkingV1().IPInstances(pod.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.SetAsLabelSelector(map[string]string{
			constants.LabelPod: pod.Name,
		}).String(),
	}); err != nil {
		return err
	}

	for i := range ipInstanceList.Items {
		if err = w.deleteIP(pod.Namespace, ipInstanceList.Items[i].Name); err != nil {
			return err
		}
	}

	return w.releaseIPFromPod(pod)
}

func (w *Worker) IPRecycle(namespace string, ip *ipamtypes.IP) (err error) {
	return w.deleteIP(namespace, toDNSLabelFormat(ip))
}

func (w *Worker) IPUnBind(namespace, ip string) (err error) {
	patchBody := `{"metadata":{"finalizers":null}}`
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = w.hybridnetClient.NetworkingV1().IPInstances(namespace).Patch(context.TODO(), ip, types.MergePatchType, []byte(patchBody), metav1.PatchOptions{})
		return err
	})
}

func (w *Worker) SyncNetworkStatus(name, nodeList, subnetList string) (err error) {
	patchBody := fmt.Sprintf(
		`{"status":{"nodeList":%s,"subnetList":%s}}`,
		nodeList,
		subnetList,
	)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = w.hybridnetClient.NetworkingV1().Networks().Patch(context.TODO(), name, types.MergePatchType, []byte(patchBody), metav1.PatchOptions{}, "status")
		return err
	})
}

func (w *Worker) SyncNetworkUsage(name string, usage *ipamtypes.Usage) (err error) {
	patchBody := fmt.Sprintf(
		`{"status":{"lastAllocatedSubnet":%q,"statistics":{"total":%d,"used":%d,"available":%d}}}`,
		usage.LastAllocation,
		usage.Total,
		usage.Used,
		usage.Available,
	)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = w.hybridnetClient.NetworkingV1().Networks().Patch(context.TODO(), name, types.MergePatchType, []byte(patchBody), metav1.PatchOptions{}, "status")
		return err
	})
}

func (w *Worker) SyncSubnetUsage(name string, usage *ipamtypes.Usage) (err error) {
	patchBody := fmt.Sprintf(
		`{"status":{"lastAllocatedIP":%q,"total":%d,"used":%d,"available":%d}}`,
		usage.LastAllocation,
		usage.Total,
		usage.Used,
		usage.Available,
	)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = w.hybridnetClient.NetworkingV1().Subnets().Patch(context.TODO(), name, types.MergePatchType, []byte(patchBody), metav1.PatchOptions{}, "status")
		return err
	})
}

func (w *Worker) updateIPStatus(ip *networkingv1.IPInstance, nodeName, podName, podNamespace, phase string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := w.hybridnetClient.NetworkingV1().IPInstances(ip.Namespace).Patch(context.TODO(),
			ip.Name,
			types.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"status":{"podName":%q,"podNamespace":%q,"nodeName":%q,"phase":%q}}`,
				podName,
				podNamespace,
				nodeName,
				phase,
			)),
			metav1.PatchOptions{},
			"status",
		)
		return err
	})
}

func (w *Worker) createIP(pod *v1.Pod, ip *ipamtypes.IP) (ipIns *networkingv1.IPInstance, err error) {
	return w.createIPWithMAC(pod, ip, mac.GenerateMAC().String())
}

func (w *Worker) createIPWithMAC(pod *v1.Pod, ip *ipamtypes.IP, macAddr string) (ipIns *networkingv1.IPInstance, err error) {
	owner := strategy.GetKnownOwnReference(pod)
	if owner == nil {
		owner = newControllerRef(pod, v1.SchemeGroupVersion.WithKind("Pod"))
	}

	ipInstance := &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       toDNSLabelFormat(ip),
			Namespace:  pod.Namespace,
			Finalizers: []string{constants.FinalizerIPAllocated},
			Labels: map[string]string{
				constants.LabelSubnet:  ip.Subnet,
				constants.LabelNetwork: ip.Network,
				constants.LabelNode:    pod.Spec.NodeName,
				constants.LabelPod:     pod.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: networkingv1.IPInstanceSpec{
			Network: ip.Network,
			Subnet:  ip.Subnet,
			Address: networkingv1.Address{
				Version: extractIPVersion(ip),
				IP:      ip.Address.String(),
				Gateway: ip.Gateway.String(),
				NetID:   ip.NetID,
				MAC:     macAddr,
			},
		},
	}

	return w.hybridnetClient.NetworkingV1().IPInstances(pod.Namespace).Create(context.TODO(), ipInstance, metav1.CreateOptions{})
}

func (w *Worker) deleteIP(namespace, name string) error {
	return w.hybridnetClient.NetworkingV1().IPInstances(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (w *Worker) getIP(namespace string, ip *ipamtypes.IP) (*networkingv1.IPInstance, error) {
	return w.hybridnetClient.NetworkingV1().IPInstances(namespace).Get(context.TODO(), toDNSLabelFormat(ip), metav1.GetOptions{})
}

// patchIPtoPod will patch a specified IP annotation into pod
func (w *Worker) patchIPtoPod(pod *v1.Pod, ip *ipamtypes.IP) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := w.kubeClient.CoreV1().Pods(pod.Namespace).Patch(context.TODO(),
			pod.Name,
			types.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"metadata":{"annotations":{%q:%q}}}`,
				constants.AnnotationIP,
				marshal(ip),
			)),
			metav1.PatchOptions{},
		)
		return err
	})
}

// releaseIPFromPod will remove the specified IP annotation from pod
func (w *Worker) releaseIPFromPod(pod *v1.Pod) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := w.kubeClient.CoreV1().Pods(pod.Namespace).Patch(context.TODO(),
			pod.Name,
			types.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"metadata":{"annotations":{%q:null}}}`,
				constants.AnnotationIP,
			)),
			metav1.PatchOptions{},
		)
		return err
	})
}

func (w *Worker) patchIPLabels(ip *networkingv1.IPInstance, podName, nodeName string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := w.hybridnetClient.NetworkingV1().IPInstances(ip.Namespace).Patch(context.TODO(),
			ip.Name,
			types.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"metadata":{"labels":{"%s":%q,"%s":%q}}}`,
				constants.LabelNode,
				nodeName,
				constants.LabelPod,
				podName,
			)),
			metav1.PatchOptions{},
		)
		return err
	})
}

func marshal(ip *ipamtypes.IP) string {
	bytes, _ := json.Marshal(ip)
	return string(bytes)
}

func toDNSLabelFormat(ip *ipamtypes.IP) string {
	if !ip.IsIPv6() {
		return strings.ReplaceAll(ip.Address.IP.String(), ".", "-")
	}

	return strings.ReplaceAll(unifyIPv6AddressString(ip.Address.IP.String()), ":", "-")
}

func newControllerRef(owner metav1.Object, gvk schema.GroupVersionKind) *metav1.OwnerReference {
	blockOwnerDeletion := false
	isController := true
	return &metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func extractIPVersion(ip *ipamtypes.IP) networkingv1.IPVersion {
	if ip.IsIPv6() {
		return networkingv1.IPv6
	}
	return networkingv1.IPv4
}

// unifyIPv6AddressString will help to extend the squashed sections in IPv6 address string,
// eg, 234e:0:4567::5f will be unified to 234e:0:4567:0:0:0:0:5f
func unifyIPv6AddressString(ip string) string {
	const maxSectionCount = 8

	if sectionCount := strings.Count(ip, ":") + 1; sectionCount < maxSectionCount {
		var separators = []string{":", ":"}
		for ; sectionCount < maxSectionCount; sectionCount++ {
			separators = append(separators, ":")
		}
		return strings.ReplaceAll(ip, "::", strings.Join(separators, "0"))
	}

	return ip
}
