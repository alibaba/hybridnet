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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/mac"
)

const podKind = "Pod"

type Worker struct {
	client.Client
}

func NewWorker(client client.Client) *Worker {
	return &Worker{
		Client: client,
	}
}

func (w *Worker) Couple(pod *corev1.Pod, ip *ipamtypes.IP) (err error) {
	_, err = w.createIP(pod, ip)
	return
}

func (w *Worker) ReCouple(pod *corev1.Pod, ip *ipamtypes.IP) (err error) {
	_, err = w.createOrUpdateIPWithMac(pod, ip, "")
	return
}

func (w *Worker) DeCouple(pod *corev1.Pod) (err error) {
	var ipInstanceList = &networkingv1.IPInstanceList{}
	if err = w.List(context.TODO(),
		ipInstanceList,
		client.MatchingLabels{
			constants.LabelPod: pod.Name,
		},
		client.InNamespace(pod.Namespace),
	); err != nil {
		return err
	}

	var deleteFuncs []func() error
	for i := range ipInstanceList.Items {
		deleteFuncs = append(deleteFuncs, func() error {
			return w.deleteIP(pod.Namespace, ipInstanceList.Items[i].Name)
		})
	}

	return errors.AggregateGoroutines(deleteFuncs...)
}

func (w *Worker) IPReserve(pod *corev1.Pod) (err error) {
	var ipInstanceList = &networkingv1.IPInstanceList{}
	if err = w.List(context.TODO(),
		ipInstanceList,
		client.MatchingLabels{
			constants.LabelPod: pod.Name,
		},
		client.InNamespace(pod.Namespace),
	); err != nil {
		return err
	}

	var reserveFuncs []func() error
	for i := range ipInstanceList.Items {
		var ipIns = &ipInstanceList.Items[i]
		reserveFuncs = append(reserveFuncs, func() error {
			_, err := controllerutil.CreateOrPatch(context.TODO(), w, ipIns, func() error {
				reserveIPInstance(ipIns)
				return nil
			})
			return err
		})
	}

	return errors.AggregateGoroutines(reserveFuncs...)
}

func (w *Worker) IPRecycle(namespace string, ip *ipamtypes.IP) (err error) {
	return w.deleteIP(namespace, toDNSLabelFormat(ip))
}

func (w *Worker) IPUnBind(namespace, ip string) (err error) {
	patchBody := `{"metadata":{"finalizers":null}}`
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return w.Patch(context.TODO(),
			&networkingv1.IPInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      ip,
				},
			},
			client.RawPatch(
				types.MergePatchType,
				[]byte(patchBody),
			),
		)
	})
}

func (w *Worker) createIP(pod *corev1.Pod, ip *ipamtypes.IP) (ipIns *networkingv1.IPInstance, err error) {
	return w.createIPWithMAC(pod, ip, mac.GenerateMAC().String())
}

func (w *Worker) createIPWithMAC(pod *corev1.Pod, ip *ipamtypes.IP, macAddr string) (ipIns *networkingv1.IPInstance, err error) {
	ipInstance := &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toDNSLabelFormat(ip),
			Namespace: pod.Namespace,
		},
	}

	fillIPInstance(ipInstance, ip, pod, macAddr)

	return ipInstance, w.Create(context.TODO(), ipInstance)
}

func (w *Worker) createOrUpdateIPWithMac(pod *corev1.Pod, ip *ipamtypes.IP, macAddr string) (ipIns *networkingv1.IPInstance, err error) {
	var ipInstance = &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toDNSLabelFormat(ip),
			Namespace: pod.Namespace,
		},
	}

	_, err = controllerutil.CreateOrPatch(context.TODO(), w, ipInstance, func() error {
		if !ipInstance.DeletionTimestamp.IsZero() {
			return fmt.Errorf("ip instance %s/%s is deleting, can not be updated", ipInstance.Namespace, ipInstance.Name)
		}

		// mac address will be regenerated if reused ipInstance was deleted unexpectedly
		fillIPInstance(ipInstance, ip, pod, macAddr)
		return nil
	})

	return ipInstance, err
}

func fillIPInstance(ipIns *networkingv1.IPInstance, ip *ipamtypes.IP, pod *corev1.Pod, macAddr string) {
	ipIns.Finalizers = []string{constants.FinalizerIPAllocated}

	if len(ipIns.Labels) == 0 {
		ipIns.Labels = map[string]string{}
	}
	ipIns.Labels[constants.LabelVersion] = networkingv1.IPInstanceLatestVersion
	ipIns.Labels[constants.LabelSubnet] = ip.Subnet
	ipIns.Labels[constants.LabelNetwork] = ip.Network
	ipIns.Labels[constants.LabelNode] = pod.Spec.NodeName
	ipIns.Labels[constants.LabelPod] = pod.Name
	ipIns.Labels[constants.LabelPodUID] = string(pod.UID)

	owner := strategy.GetKnownOwnReference(pod)
	if owner == nil {
		owner = newControllerRef(pod, corev1.SchemeGroupVersion.WithKind(podKind))
	}
	ipIns.OwnerReferences = []metav1.OwnerReference{*owner}

	ipIns.Spec.Network = ip.Network
	ipIns.Spec.Subnet = ip.Subnet

	if len(ipIns.Spec.Address.IP) == 0 {
		ipIns.Spec.Address = networkingv1.Address{
			Version: extractIPVersion(ip),
			IP:      ip.Address.String(),
			NetID:   uint32PtoInt32P(ip.NetID),
		}

		if len(macAddr) > 0 {
			ipIns.Spec.Address.MAC = macAddr
		}
		if len(ipIns.Spec.Address.MAC) == 0 {
			ipIns.Spec.Address.MAC = mac.GenerateMAC().String()
		}

		if ip.Gateway != nil {
			ipIns.Spec.Address.Gateway = ip.Gateway.String()
		}
	}

	ipIns.Spec.Binding = networkingv1.Binding{
		ReferredObject: networkingv1.ObjectMeta{
			Kind: owner.Kind,
			Name: owner.Name,
			UID:  owner.UID,
		},
		NodeName: pod.Spec.NodeName,
		PodUID:   pod.UID,
		PodName:  pod.Name,
	}

	if strategy.OwnByStatefulWorkload(pod) {
		ipIns.Spec.Binding.Stateful = &networkingv1.StatefulInfo{
			Index: intToInt32P(utils.GetIndexFromName(pod.Name)),
		}
	}
}

// reserveIPInstance means this IPInstance does not belong to a specific
// node and a pod with specific UID
func reserveIPInstance(ipIns *networkingv1.IPInstance) {
	// clean pod uid & node info means this IP is not being used by any pod
	ipIns.Spec.Binding.NodeName = ""
	ipIns.Spec.Binding.PodUID = ""
	delete(ipIns.Labels, constants.LabelNode)
	delete(ipIns.Labels, constants.LabelPodUID)

	// TODO: clean status
}

func (w *Worker) deleteIP(namespace, name string) error {
	return w.Delete(context.TODO(), &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	})
}

func (w *Worker) getIP(namespace string, ip *ipamtypes.IP) (*networkingv1.IPInstance, error) {
	var ipInstance = &networkingv1.IPInstance{}
	if err := w.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: toDNSLabelFormat(ip)}, ipInstance); err != nil {
		return nil, err
	}
	return ipInstance, nil
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

func uint32PtoInt32P(in *uint32) *int32 {
	if in == nil {
		return nil
	}

	out := int32(*in)
	return &out
}

func intToInt32P(in int) *int32 {
	out := int32(in)
	return &out
}
