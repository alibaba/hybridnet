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
	"math"
	"net"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	networkingv1 "github.com/alibaba/hybridnet/pkg/client/listers/networking/v1"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/transform"
)

var (
	statefulWorkloadKindVar  = "StatefulSet"
	statelessWorkloadKindVar = ""
	StatefulWorkloadKind     map[string]bool
	StatelessWorkloadKind    map[string]bool
	DefaultIPRetain          bool
)

func init() {
	pflag.BoolVar(&DefaultIPRetain, "default-ip-retain", true, "Whether pod IP of stateful workloads will be retained by default.")
	pflag.StringVar(&statefulWorkloadKindVar, "stateful-workload-kinds", statefulWorkloadKindVar, `stateful workload kinds to use strategic IP allocation,`+
		`eg: "StatefulSet,AdvancedStatefulSet", default: "StatefulSet"`)
	pflag.StringVar(&statelessWorkloadKindVar, "stateless-workload-kinds", statelessWorkloadKindVar, "stateless workload kinds to use strategic IP allocation,"+
		`eg: "ReplicaSet", default: ""`)

	StatefulWorkloadKind = make(map[string]bool)
	StatelessWorkloadKind = make(map[string]bool)

	for _, kind := range strings.Split(statefulWorkloadKindVar, ",") {
		if len(kind) > 0 {
			StatefulWorkloadKind[kind] = true
			klog.Infof("[strategy] Adding kind %s to known stateful workloads", kind)
		}
	}

	for _, kind := range strings.Split(statelessWorkloadKindVar, ",") {
		if len(kind) > 0 {
			StatelessWorkloadKind[kind] = true
			klog.Infof("[strategy] Adding kind %s to known stateless workloads", kind)
		}
	}
}

func OwnByStatefulWorkload(pod *v1.Pod) bool {
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return false
	}

	return StatefulWorkloadKind[ref.Kind]
}

func OwnByStatelessWorkload(pod *v1.Pod) bool {
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return false
	}

	return StatelessWorkloadKind[ref.Kind]
}

func GetKnownOwnReference(pod *v1.Pod) *metav1.OwnerReference {
	// only support stateful workloads
	if OwnByStatefulWorkload(pod) {
		return metav1.GetControllerOf(pod)
	}
	return nil
}

func GetIPByPod(ipLister networkingv1.IPInstanceLister, pod *v1.Pod) (string, error) {
	ips, err := ipLister.IPInstances(pod.Namespace).List(labels.Everything())
	if err != nil {
		return "", err
	}

	for _, ip := range ips {
		// terminating ipInstance should not be picked up
		if ip.Status.PodName == pod.Name && ip.DeletionTimestamp == nil {
			ipStr, _ := toIPFormat(ip.Name)
			return ipStr, nil
		}
	}

	return "", nil
}

func GetIPsByPod(ipLister networkingv1.IPInstanceLister, pod *v1.Pod) ([]string, error) {
	ips, err := ipLister.IPInstances(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var v4, v6 []string
	for _, ip := range ips {
		// terminating ipInstance should not be picked up
		if ip.Status.PodName == pod.Name && ip.DeletionTimestamp == nil {
			ipStr, isIPv6 := toIPFormat(ip.Name)
			if isIPv6 {
				v6 = append(v6, ipStr)
			} else {
				v4 = append(v4, ipStr)
			}
		}
	}

	return append(v4, v6...), nil
}

func GetAllocatedIPsByPod(ipLister networkingv1.IPInstanceLister, pod *v1.Pod) ([]*types.IP, error) {
	ips, err := ipLister.IPInstances(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var allocatedIPs []*types.IP
	for _, ip := range ips {
		// terminating ipInstance should not be picked up
		if ip.Status.PodName == pod.Name && ip.DeletionTimestamp == nil {
			allocatedIPs = append(allocatedIPs, transform.TransferIPInstanceForIPAM(ip))
		}
	}

	return allocatedIPs, nil
}

func GetIndexFromName(name string) int {
	nameSlice := strings.Split(name, "-")
	indexStr := nameSlice[len(nameSlice)-1]

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return math.MaxInt32
	}
	return index
}

func toIPFormat(name string) (string, bool) {
	const IPv6SeparatorCount = 7
	if isIPv6 := strings.Count(name, "-") == IPv6SeparatorCount; isIPv6 {
		return net.ParseIP(strings.ReplaceAll(name, "-", ":")).String(), true
	}
	return strings.ReplaceAll(name, "-", "."), false
}
