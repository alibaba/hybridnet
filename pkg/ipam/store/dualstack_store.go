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
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/mac"
)

type DualStackWorker struct {
	client.Client
	worker *Worker
}

func NewDualStackWorker(c client.Client) *DualStackWorker {
	return &DualStackWorker{
		Client: c,
		worker: &Worker{
			Client: c,
		},
	}
}

func (d *DualStackWorker) Couple(pod *v1.Pod, IPs []*types.IP) (err error) {
	var createdNames []string
	defer func() {
		if err != nil {
			for _, name := range createdNames {
				_ = d.worker.deleteIP(pod.Namespace, name)
			}
		}
	}()

	var globalMac = mac.GenerateMAC().String()
	for _, ip := range IPs {
		var ipIns *networkingv1.IPInstance
		if ipIns, err = d.worker.createIPWithMAC(pod, ip, globalMac); err != nil {
			return err
		}
		createdNames = append(createdNames, ipIns.Name)
	}

	return nil
}

func (d *DualStackWorker) ReCouple(pod *v1.Pod, IPs []*types.IP) (err error) {
	var globalMac = mac.GenerateMAC().String()
	for _, ip := range IPs {
		var ipIns *networkingv1.IPInstance
		if ipIns, err = d.worker.getIP(pod.Namespace, ip); err != nil {
			// ignore the not-found error
			if err = client.IgnoreNotFound(err); err == nil {
				continue
			}
			return
		}

		// fetch MAC address from paired ip instance created and try to reuse it
		globalMac = ipIns.Spec.Address.MAC
		break
	}

	for _, ip := range IPs {
		if _, err = d.worker.createOrUpdateIPWithMac(pod, ip, globalMac); err != nil {
			return
		}
	}

	return
}

func (d *DualStackWorker) DeCouple(pod *v1.Pod) (err error) {
	return d.worker.DeCouple(pod)
}

func (d *DualStackWorker) IPReserve(pod *v1.Pod) (err error) {
	return d.worker.IPReserve(pod)
}

func (d *DualStackWorker) IPRecycle(namespace string, ip *types.IP) (err error) {
	return d.worker.IPRecycle(namespace, ip)
}

func (d *DualStackWorker) IPUnBind(namespace, ip string) (err error) {
	return d.worker.IPUnBind(namespace, ip)
}
