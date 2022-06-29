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

package strategy

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kubevirtv1 "kubevirt.io/api/core/v1"
)

var (
	StatefulWorkloadKinds []string
	DefaultIPRetain       bool
)

var (
	statefulOnce            sync.Once
	statefulWorkloadKindSet sets.String
)

func init() {
	pflag.BoolVar(&DefaultIPRetain, "default-ip-retain", true, "Whether pod IP of stateful workloads will be retained by default.")
	pflag.StringSliceVar(&StatefulWorkloadKinds, "stateful-workload-kinds", []string{"StatefulSet"}, `stateful workload kinds to use strategic IP allocation,`+
		`eg: "StatefulSet,AdvancedStatefulSet", default: "StatefulSet"`)
}

func OwnByStatefulWorkload(obj client.Object) bool {
	ref := metav1.GetControllerOf(obj)
	if ref == nil {
		return false
	}

	statefulOnce.Do(func() {
		statefulWorkloadKindSet = sets.NewString(StatefulWorkloadKinds...)
		logger := log.Log.WithName("strategy")
		logger.Info("Adding known stateful workloads", "Kinds", StatefulWorkloadKinds)
	})

	return statefulWorkloadKindSet.Has(ref.Kind)
}

func OwnByVirtualMachineInstance(obj client.Object) (bool, string) {
	ref := metav1.GetControllerOf(obj)
	if ref == nil {
		return false, ""
	}

	if ref.Kind != kubevirtv1.VirtualMachineInstanceGroupVersionKind.Kind {
		return false, ""
	}

	return true, ref.Name
}

func OwnByVirtualMachine(ctx context.Context, pod *v1.Pod, client client.Reader) (bool, string, *metav1.OwnerReference, error) {
	ownByVMI, vmiName := OwnByVirtualMachineInstance(pod)
	if !ownByVMI {
		return false, "", nil, nil
	}

	vmi := &kubevirtv1.VirtualMachineInstance{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      vmiName,
		Namespace: pod.Namespace,
	}, vmi); err != nil {
		return false, "", nil, fmt.Errorf("failed to get kubevirt VMI %v/%v: %v", pod.Namespace, vmiName, err)
	}

	vmiRef := metav1.GetControllerOf(vmi)
	if vmiRef == nil {
		return false, "", nil, nil
	}

	if vmiRef.Kind != kubevirtv1.VirtualMachineGroupVersionKind.Kind {
		return false, "", nil, nil
	}

	ifBlockOwnerDeletion := false
	vmiRef.BlockOwnerDeletion = &ifBlockOwnerDeletion

	return true, vmiRef.Name, vmiRef, nil
}

func GetKnownOwnReference(pod *v1.Pod) *metav1.OwnerReference {
	// only support stateful workloads
	if OwnByStatefulWorkload(pod) {
		return metav1.GetControllerOf(pod)
	}
	return nil
}
