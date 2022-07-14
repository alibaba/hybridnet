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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/ipam"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/ipam/utils"
	"github.com/alibaba/hybridnet/pkg/utils/mac"
)

const podKind = "Pod"

var _ ipam.Store = &crdStore{}

// crdStore is an implementation of Store based on some Custom Resource Definitions
type crdStore struct {
	client.Client
}

func NewCRDStore(c client.Client) ipam.Store {
	return &crdStore{
		Client: c,
	}
}

// Couple will create related IPInstances bind to a specified pod
func (s *crdStore) Couple(ctx context.Context, pod *corev1.Pod, IPs []*ipamtypes.IP, opts ...ipamtypes.CoupleOption) (err error) {
	var (
		createdNames []string
		options      = &ipamtypes.CoupleOptions{}
	)

	// parse options
	options.ApplyOptions(opts)

	defer func() {
		if err != nil {
			for _, name := range createdNames {
				_ = s.deleteIPInstance(ctx, pod.Namespace, name)
			}
		}
	}()

	var unifiedMACAddr string
	if options.SpecifiedMACAddress.IsEmpty() {
		unifiedMACAddr = mac.GenerateMAC().String()
	} else {
		unifiedMACAddr = string(options.SpecifiedMACAddress)
	}
	for _, ip := range IPs {
		var ipInstance *networkingv1.IPInstance
		if ipInstance, err = s.createIPInstance(ctx, pod, ip, unifiedMACAddr, options.OwnerReference, options.AdditionalLabels); err != nil {
			return err
		}
		createdNames = append(createdNames, ipInstance.Name)
	}

	return nil
}

// ReCouple will create or update related IPInstances, and force them redirect to a specified pod
func (s *crdStore) ReCouple(ctx context.Context, pod *corev1.Pod, IPs []*ipamtypes.IP, opts ...ipamtypes.ReCoupleOption) (err error) {
	var (
		unifiedMACAddr string
		options        = &ipamtypes.ReCoupleOptions{}
	)

	// parse options
	options.ApplyOptions(opts)

	// If MAC address is specified in options, use it as unified MAC address.
	if !options.SpecifiedMACAddress.IsEmpty() {
		unifiedMACAddr = string(options.SpecifiedMACAddress)
	}

	// If there is no specified MAC address in options, and pod will be recoupled with
	// multi IPs, a unified MAC address should be reused.
	if options.SpecifiedMACAddress.IsEmpty() && len(IPs) > 1 {
		for _, ip := range IPs {
			var ipInstance *networkingv1.IPInstance
			if ipInstance, err = s.getIPInstance(ctx, pod.Namespace, ip); err != nil {
				// ignore the not-found error
				if err = client.IgnoreNotFound(err); err == nil {
					continue
				}
				return
			}

			// fetch valid MAC address from created ip instances and try to reuse it
			unifiedMACAddr = ipInstance.Spec.Address.MAC
			break
		}
	}

	// If no valid MAC address reused or specified in options, create a new one.
	if len(unifiedMACAddr) == 0 {
		unifiedMACAddr = mac.GenerateMAC().String()
	}

	for _, ip := range IPs {
		if _, err = s.createOrUpdateIPInstance(ctx, pod, ip, unifiedMACAddr, options.OwnerReference, options.AdditionalLabels); err != nil {
			return
		}
	}

	return
}

// DeCouple will release(remove) related IPInstances of a specified pod
func (s *crdStore) DeCouple(ctx context.Context, pod *corev1.Pod) (err error) {
	var ipInstanceList = &networkingv1.IPInstanceList{}
	if err = s.List(ctx,
		ipInstanceList,
		client.MatchingLabels{
			constants.LabelPod: pod.Name,
		},
		client.InNamespace(pod.Namespace),
	); err != nil {
		return err
	}

	var deleteFunctions []func() error
	for i := range ipInstanceList.Items {
		var ipInstanceName = ipInstanceList.Items[i].Name
		deleteFunctions = append(deleteFunctions, func() error {
			return s.deleteIPInstance(ctx, pod.Namespace, ipInstanceName)
		})
	}

	return errors.AggregateGoroutines(deleteFunctions...)
}

// IPReserve will change IPInstances to reservation status of a specified pod
func (s *crdStore) IPReserve(ctx context.Context, pod *corev1.Pod, opts ...ipamtypes.ReserveOption) (err error) {
	var options = &ipamtypes.ReserveOptions{}

	// parse options
	options.ApplyOptions(opts)

	var ipInstanceList = &networkingv1.IPInstanceList{}
	if err = s.List(ctx,
		ipInstanceList,
		client.MatchingLabels{
			constants.LabelPod: pod.Name,
		},
		client.InNamespace(pod.Namespace),
	); err != nil {
		return err
	}

	var reserveFunctions []func() error
	for i := range ipInstanceList.Items {
		var ipInstance = &ipInstanceList.Items[i]
		// if ip instance is terminating, no need to reserve it,
		// just skip
		if ipInstance.DeletionTimestamp != nil {
			continue
		}
		reserveFunctions = append(reserveFunctions, func() error {
			return s.reserveIPInstance(ctx, ipInstance, options.DropPodName)
		})
	}

	return errors.AggregateGoroutines(reserveFunctions...)
}

// IPRecycle will remove a specified IPInstance by name
func (s *crdStore) IPRecycle(ctx context.Context, namespace string, ip *ipamtypes.IP) (err error) {
	return s.deleteIPInstance(ctx, namespace, utils.ToDNSLabelFormatName(ip))
}

// IPUnBind will be called after IPInstance garbage collection in memory to announce
// this IPInstance would be deleted from persist storage
func (s *crdStore) IPUnBind(ctx context.Context, namespace, ip string) (err error) {
	return s.removeFinalizerOfIPInstance(ctx, namespace, ip)
}

// createIPInstance will create an IPInstance by pod info, ip info and mac address
func (s *crdStore) createIPInstance(ctx context.Context, pod *corev1.Pod, ip *ipamtypes.IP, macAddr string, ownerReference *metav1.OwnerReference, additionalLabels map[string]string) (ipIns *networkingv1.IPInstance, err error) {
	// Check MAC address collision with existing ip instances of other pods.
	if len(macAddr) > 0 {
		err = s.checkMACAddressCollision(pod, macAddr)
		if err != nil {
			return nil, fmt.Errorf("fail to check MAC address collision %v", err)
		}
	}

	ipInstance := &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ToDNSLabelFormatName(ip),
			Namespace: pod.Namespace,
		},
	}

	assembleIPInstance(ipInstance, ip, pod, macAddr, ownerReference, additionalLabels)

	return ipInstance, s.Create(ctx, ipInstance)
}

// createOrUpdateIPInstance will create or update an IPInstance by pod info, ip info and mac address
func (s *crdStore) createOrUpdateIPInstance(ctx context.Context, pod *corev1.Pod, ip *ipamtypes.IP, macAddr string, ownerReference *metav1.OwnerReference, additionalLabels map[string]string) (ipIns *networkingv1.IPInstance, err error) {
	// Check MAC address collision with existing ip instances of other pods.
	if len(macAddr) > 0 {
		err = s.checkMACAddressCollision(pod, macAddr)
		if err != nil {
			return nil, fmt.Errorf("fail to check MAC address collision %v", err)
		}
	}

	var ipInstance = &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ToDNSLabelFormatName(ip),
			Namespace: pod.Namespace,
		},
	}

	_, err = controllerutil.CreateOrPatch(ctx, s, ipInstance, func() error {
		if !ipInstance.DeletionTimestamp.IsZero() {
			return fmt.Errorf("ip instance %s/%s is deleting, can not be updated", ipInstance.Namespace, ipInstance.Name)
		}

		// mac address will be regenerated if reused ipInstance was deleted unexpectedly
		assembleIPInstance(ipInstance, ip, pod, macAddr, ownerReference, additionalLabels)
		return nil
	})

	return ipInstance, err
}

func (s *crdStore) checkMACAddressCollision(pod *corev1.Pod, macAddr string) (err error) {
	ipInstanceList := &networkingv1.IPInstanceList{}
	if err = s.List(context.TODO(), ipInstanceList, client.MatchingFields{ipamtypes.IndexerFieldMAC: macAddr}); err != nil {
		return fmt.Errorf("unable to list ip instances by indexer MAC %s: %v", macAddr, err)
	}
	for _, ipInstance := range ipInstanceList.Items {
		if !ipInstance.DeletionTimestamp.IsZero() {
			continue
		}
		if ipInstance.Status.PodNamespace != pod.GetNamespace() || ipInstance.Status.PodName != pod.GetName() {
			return fmt.Errorf("specified mac address %s is in conflict with existing ip instance %s/%s", macAddr, ipInstance.Namespace, ipInstance.Name)
		}
	}
	return nil
}

// deleteIPInstance will remove an IPInstance by namespace and name
func (s *crdStore) deleteIPInstance(ctx context.Context, namespace, name string) error {
	return s.Delete(ctx, &networkingv1.IPInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	})
}

// getIPInstance will get an IPInstance by namespace and name
func (s *crdStore) getIPInstance(ctx context.Context, namespace string, ip *ipamtypes.IP) (*networkingv1.IPInstance, error) {
	var ipInstance = &networkingv1.IPInstance{}
	if err := s.Get(ctx, types.NamespacedName{Namespace: namespace, Name: utils.ToDNSLabelFormatName(ip)}, ipInstance); err != nil {
		return nil, err
	}
	return ipInstance, nil
}

// reserveIPInstance means this IPInstance does not belong to a specific
// node and a pod with specific UID, also the status is meaningless
func (s *crdStore) reserveIPInstance(ctx context.Context, ipInstance *networkingv1.IPInstance, dropPodName bool) error {
	_, err := controllerutil.CreateOrPatch(ctx, s, ipInstance,
		// update both spec and status for reservation
		func() error {
			// if ip instance is terminating, no need to reserve it
			if ipInstance.DeletionTimestamp != nil {
				return nil
			}

			// clean pod uid & node info means this IP is not being used by any pod
			ipInstance.Spec.Binding.NodeName = ""
			ipInstance.Spec.Binding.PodUID = ""
			delete(ipInstance.Labels, constants.LabelNode)
			delete(ipInstance.Labels, constants.LabelPodUID)

			// clean pod name if set
			if dropPodName {
				ipInstance.Spec.Binding.PodName = ""
				delete(ipInstance.Labels, constants.LabelPod)
			}

			// refresh status
			ipInstance.Status.PodName = ""
			ipInstance.Status.PodNamespace = ""
			ipInstance.Status.NodeName = ""
			ipInstance.Status.SandboxID = ""
			ipInstance.Status.UpdateTimestamp = metav1.Now()
			return nil
		},
	)
	return err
}

// removeFinalizerOfIPInstance will remove the blocking finalizer of a specified IPInstance
func (s *crdStore) removeFinalizerOfIPInstance(ctx context.Context, namespace, name string) error {
	patchBody := `{"metadata":{"finalizers":null}}`
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return s.Patch(ctx,
			&networkingv1.IPInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
			},
			client.RawPatch(
				types.MergePatchType,
				[]byte(patchBody),
			),
		)
	})
}

// assembleIPInstance will assemble the spec of IPInstance with provided inputs,
// including pod, ip info and mac address
func assembleIPInstance(ipIns *networkingv1.IPInstance, ip *ipamtypes.IP, pod *corev1.Pod, macAddr string, ownerReference *metav1.OwnerReference, additionalLabels map[string]string) {
	// finalizer will block deletion for garbage collection
	ipIns.Finalizers = []string{constants.FinalizerIPAllocated}

	// labels will help quick search by label-selecting
	if len(ipIns.Labels) == 0 {
		ipIns.Labels = map[string]string{}
	}
	ipIns.Labels[constants.LabelVersion] = networkingv1.IPInstanceLatestVersion
	ipIns.Labels[constants.LabelSubnet] = ip.Subnet
	ipIns.Labels[constants.LabelNetwork] = ip.Network
	ipIns.Labels[constants.LabelNode] = pod.Spec.NodeName
	ipIns.Labels[constants.LabelPod] = pod.Name
	ipIns.Labels[constants.LabelPodUID] = string(pod.UID)

	// additional labels will be patched
	// NOTICE: additional labels will take higher priority than built-in lables
	for k, v := range additionalLabels {
		ipIns.Labels[k] = v
	}

	// set owner reference for IPInstance
	// if one assigned owner reference passed from upstream, ust it directly
	// otherwise one known owner reference will get parsed by kind, if still
	// no owner reference got, fall back to owner reference of this pod
	var owner *metav1.OwnerReference
	if ownerReference != nil {
		owner = ownerReference
	} else {
		if owner = strategy.GetKnownOwnReference(pod); owner == nil {
			owner = utils.NewControllerRef(pod, corev1.SchemeGroupVersion.WithKind(podKind), true, false)
		}
	}

	ipIns.OwnerReferences = []metav1.OwnerReference{*owner}

	// parent network and subnet name
	ipIns.Spec.Network = ip.Network
	ipIns.Spec.Subnet = ip.Subnet

	// ip spec is composed by ip info and mac address
	if len(ipIns.Spec.Address.IP) == 0 {
		ipIns.Spec.Address = networkingv1.Address{
			Version: utils.ExtractIPVersion(ip),
			IP:      ip.Address.String(),
			NetID:   utils.Uint32PtoInt32P(ip.NetID),
		}

		// if mac address specified, override this original value
		// if mac address still empty after overriding, generate a new one
		if len(macAddr) > 0 {
			ipIns.Spec.Address.MAC = macAddr
		}
		if len(ipIns.Spec.Address.MAC) == 0 {
			ipIns.Spec.Address.MAC = mac.GenerateMAC().String()
		}

		// gateway is optional
		if ip.Gateway != nil {
			ipIns.Spec.Address.Gateway = ip.Gateway.String()
		}
	}

	// binding point to the owner
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

	// index is the serial number of a stateful workload
	if strategy.OwnByStatefulWorkload(pod) {
		ipIns.Spec.Binding.Stateful = &networkingv1.StatefulInfo{
			Index: utils.IntToInt32P(utils.GetIndexFromName(pod.Name)),
		}
	}
}
