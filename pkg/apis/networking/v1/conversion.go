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

package v1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
)

const (
	IPInstanceV11           = "v1.1"
	IPInstanceV12           = "v1.2"
	IPInstanceLatestVersion = IPInstanceV12
)

func parseIPInstanceVersion(ipInstance *IPInstance) string {
	if len(ipInstance.Labels[constants.LabelVersion]) == 0 {
		return IPInstanceV11
	}
	return ipInstance.Labels[constants.LabelVersion]
}

func CanonicalizeIPInstance(c client.Client) (err error) {
	getPodUID := func(namespace, name string) (types.UID, error) {
		pod := &corev1.Pod{}
		if err := c.Get(context.TODO(),
			types.NamespacedName{
				Namespace: namespace,
				Name:      name,
			},
			pod); err != nil {
			if err = client.IgnoreNotFound(err); err == nil {
				return "", nil
			}
			return "", err
		}
		return pod.UID, nil
	}

	var ipInstanceList = &IPInstanceList{}
	if err = c.List(context.TODO(), ipInstanceList); err != nil {
		return err
	}

	var patchFuncs []func() error
	for i := range ipInstanceList.Items {
		var ipInstance = &ipInstanceList.Items[i]
		if parseIPInstanceVersion(ipInstance) == IPInstanceLatestVersion && !IsLegacyModel(ipInstance) {
			continue
		}

		var ipInstancePatch = client.MergeFrom(ipInstance.DeepCopy())
		if err = convertIPInstanceToLatestVersion(ipInstance, getPodUID); err != nil {
			return fmt.Errorf("unable to convert IPInstance %v/%v to latest version: %v",
				ipInstance.Namespace, ipInstance.Name, err)
		}

		patchFuncs = append(patchFuncs, func() error {
			return retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return c.Patch(context.TODO(), ipInstance, ipInstancePatch)
			})
		})
	}

	return errors.AggregateGoroutines(patchFuncs...)
}

func convertIPInstanceToLatestVersion(ipIns *IPInstance, getPodUID func(namespace, name string) (types.UID, error)) error {
	if len(ipIns.OwnerReferences) != 1 {
		return fmt.Errorf("unexpected owner reference %v", ipIns.OwnerReferences)
	}

	isStateful, owner, isReserved, bindingPodName, bindingNodeName := strategy.OwnByStatefulWorkload(ipIns),
		ipIns.OwnerReferences[0],
		IsReserved(ipIns),
		FetchBindingPodName(ipIns),
		FetchBindingNodeName(ipIns)

	ipIns.Spec.Binding = Binding{
		ReferredObject: ObjectMeta{
			Kind: owner.Kind,
			Name: owner.Name,
			UID:  owner.UID,
		},
	}

	ipIns.Labels[constants.LabelPod] = bindingPodName
	ipIns.Spec.Binding.PodName = bindingPodName

	if isReserved {
		delete(ipIns.Labels, constants.LabelNode)
		delete(ipIns.Labels, constants.LabelPodUID)
	} else {
		// get podUID and nodeName for allocated IPInstance
		podUID, err := getPodUID(ipIns.Namespace, bindingPodName)
		if err != nil {
			return fmt.Errorf("unable to get pod uid: %v", err)
		}

		ipIns.Labels[constants.LabelNode] = bindingNodeName
		ipIns.Labels[constants.LabelPodUID] = string(podUID)
		ipIns.Spec.Binding.NodeName = bindingNodeName
		ipIns.Spec.Binding.PodUID = podUID
	}

	if isStateful {
		ipIns.Spec.Binding.Stateful = &StatefulInfo{
			Index: intToInt32P(GetIndexFromName(bindingPodName)),
		}
	}

	// record IPInstance version in label
	ipIns.Labels[constants.LabelVersion] = IPInstanceLatestVersion

	return nil
}

func intToInt32P(in int) *int32 {
	out := int32(in)
	return &out
}
