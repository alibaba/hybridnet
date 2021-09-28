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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/mac"
)

type DualStackWorker struct {
	kubeClient      kubernetes.Interface
	hybridnetClient versioned.Interface
	worker          *Worker
}

func NewDualStackWorker(kubeClient kubernetes.Interface, hybridnetClient versioned.Interface) *DualStackWorker {
	return &DualStackWorker{
		kubeClient:      kubeClient,
		hybridnetClient: hybridnetClient,
		worker: &Worker{
			kubeClient:      kubeClient,
			hybridnetClient: hybridnetClient,
		},
	}
}

func (d *DualStackWorker) Couple(pod *v1.Pod, IPs []*types.IP) (err error) {
	var ipInstances []*networkingv1.IPInstance

	defer func() {
		if err != nil {
			for _, ipi := range ipInstances {
				_ = d.worker.deleteIP(ipi.Namespace, ipi.Name)
			}
		}
	}()

	var globalMac = mac.GenerateMAC().String()
	for _, ip := range IPs {
		var ipIns *networkingv1.IPInstance
		if ipIns, err = d.worker.createIPWithMAC(pod, ip, globalMac); err != nil {
			return err
		}
		ipInstances = append(ipInstances, ipIns)
	}

	for _, ipi := range ipInstances {
		if err = d.worker.updateIPStatus(ipi, pod.Spec.NodeName, pod.Name, pod.Namespace, string(networkingv1.IPPhaseUsing)); err != nil {
			return err
		}
	}

	defer func() {
		if err != nil {
			_ = d.worker.releaseIPFromPod(pod)
		}
	}()

	return d.patchIPsToPod(pod, IPs)
}

func (d *DualStackWorker) ReCouple(pod *v1.Pod, IPs []*types.IP) (err error) {
	var ipInstances []*networkingv1.IPInstance
	var missingIPs []*types.IP

	var globalMac = mac.GenerateMAC().String()
	for _, ip := range IPs {
		var ipIns *networkingv1.IPInstance
		if ipIns, err = d.worker.getIP(pod.Namespace, ip); err != nil {
			// swallow the not-found error
			if err = client.IgnoreNotFound(err); err == nil {
				missingIPs = append(missingIPs, ip)
				continue
			}
			return
		}

		ipInstances = append(ipInstances, ipIns)

		// fetch MAC address from paired ip instance created and try to reuse it
		globalMac = ipIns.Spec.Address.MAC
	}

	for _, ip := range missingIPs {
		var ipIns *networkingv1.IPInstance
		if ipIns, err = d.worker.createIPWithMAC(pod, ip, globalMac); err != nil {
			return
		}
		ipInstances = append(ipInstances, ipIns)
	}

	for _, ipi := range ipInstances {
		if err = d.worker.patchIPLabels(ipi, pod.Name, pod.Spec.NodeName); err != nil {
			return err
		}
	}

	for _, ipi := range ipInstances {
		if err = d.worker.updateIPStatus(ipi, pod.Spec.NodeName, pod.Name, pod.Namespace, string(networkingv1.IPPhaseUsing)); err != nil {
			return err
		}
	}

	return d.patchIPsToPod(pod, IPs)
}

func (d *DualStackWorker) DeCouple(pod *v1.Pod) (err error) {
	return d.worker.DeCouple(pod)
}

func (d *DualStackWorker) IPRecycle(namespace string, ip *types.IP) (err error) {
	return d.worker.IPRecycle(namespace, ip)
}

func (d *DualStackWorker) IPUnBind(namespace, ip string) (err error) {
	return d.worker.IPUnBind(namespace, ip)
}

func (d *DualStackWorker) SyncNetworkUsage(name string, usages [3]*types.Usage) (err error) {
	patchBody := fmt.Sprintf(
		`{"status":{"lastAllocatedSubnet":%q,"lastAllocatedIPv6Subnet":%q,"statistics":{"total":%d,"used":%d,"available":%d},"ipv6Statistics":{"total":%d,"used":%d,"available":%d},"dualStackStatistics":{"available":%d}}}`,
		usages[0].LastAllocation,
		usages[1].LastAllocation,
		usages[0].Total,
		usages[0].Used,
		usages[0].Available,
		usages[1].Total,
		usages[1].Used,
		usages[1].Available,
		usages[2].Available,
	)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err = d.hybridnetClient.NetworkingV1().Networks().Patch(context.TODO(), name, apitypes.MergePatchType, []byte(patchBody), metav1.PatchOptions{}, "status")
		return err
	})
}

func (d *DualStackWorker) SyncSubnetUsage(name string, usage *types.Usage) (err error) {
	return d.worker.SyncSubnetUsage(name, usage)
}

func (d *DualStackWorker) SyncNetworkStatus(name, nodes, subnets string) (err error) {
	return d.worker.SyncNetworkStatus(name, nodes, subnets)
}

func (d *DualStackWorker) patchIPsToPod(pod *v1.Pod, IPs []*types.IP) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := d.kubeClient.CoreV1().Pods(pod.Namespace).Patch(context.TODO(),
			pod.Name,
			apitypes.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"metadata":{"annotations":{%q:%q}}}`,
				constants.AnnotationIP,
				marshalIPs(IPs),
			)),
			metav1.PatchOptions{},
		)
		return err
	})
}

func marshalIPs(IPs []*types.IP) string {
	bytes, _ := json.Marshal(IPs)
	return string(bytes)
}
