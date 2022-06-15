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

package networking

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	controllerutils "github.com/alibaba/hybridnet/pkg/controllers/utils"
	"github.com/alibaba/hybridnet/pkg/utils"
)

type PodIPCache interface {
	Record(podUID types.UID, podName, namespace string, ipInstanceNames []string)
	Release(ipInstanceName, namespace string)
	Get(podName, namespace string) (bool, types.UID, []string)
}

type podAllocatedInfo struct {
	podUID          types.UID
	ipInstanceNames []string
}

type podIPCache struct {
	sync.RWMutex

	// use "name/namespace" of ip instance as key
	podToIP map[string]*podAllocatedInfo

	// use "name/namespace" of ip instance as key and "name" of pod as value
	ipToPod map[string]string

	logger logr.Logger
}

func NewPodIPCache(ctx context.Context, c client.Reader, logger logr.Logger) (PodIPCache, error) {
	cache := &podIPCache{
		podToIP: map[string]*podAllocatedInfo{},
		ipToPod: map[string]string{},
		RWMutex: sync.RWMutex{},
		logger:  logger,
	}

	ipList, err := controllerutils.ListIPInstances(ctx, c)
	if err != nil {
		return nil, err
	}

	for _, ip := range ipList.Items {
		if networkingv1.IsLegacyModel(&ip) {
			return nil, fmt.Errorf("get legacy model ip instance, if this happens more than once, " +
				"please check if the networking CRD yamls is updated to the latest v0.5 version")
		}

		podName := networkingv1.FetchBindingPodName(&ip)
		if len(podName) != 0 {
			var podUID types.UID

			if len(ip.Spec.Binding.PodUID) != 0 {
				podUID = ip.Spec.Binding.PodUID
			} else if !networkingv1.IsReserved(&ip) {
				// TODO: no longer need to get pod if all the ip instances is updated to the v1.2 version
				pod, err := controllerutils.GetPod(ctx, c, podName, ip.Namespace)
				if err != nil {
					if err = client.IgnoreNotFound(err); err != nil {
						return nil, fmt.Errorf("unable to get Pod %v for IPInstance %v: %v", podName, ip.Name, err)
					}
				}

				if pod != nil {
					podUID = pod.UID
				}
			}

			var recordedIPInstances []string
			if cache.podToIP[namespacedKey(podName, ip.Namespace)] == nil {
				recordedIPInstances = nil
			} else {
				recordedIPInstances = cache.podToIP[namespacedKey(podName, ip.Namespace)].ipInstanceNames
			}

			logger.V(1).Info("add record to init cache", "ip",
				ip.Name, "namespace", ip.Namespace, "pod", podName, "pod uid", podUID)

			// this is different from a normal Record action
			cache.podToIP[namespacedKey(podName, ip.Namespace)] = &podAllocatedInfo{
				podUID:          podUID,
				ipInstanceNames: append(recordedIPInstances, ip.Name),
			}

			cache.ipToPod[namespacedKey(ip.Name, ip.Namespace)] = podName
		}
	}

	logger.V(1).Info("finish init", "ip to pod map", cache.ipToPod)
	return cache, nil
}

func (c *podIPCache) Record(podUID types.UID, podName, namespace string, ipInstanceNames []string) {
	c.Lock()
	defer c.Unlock()

	// don't check if the pod exist, just overwrite it
	c.podToIP[namespacedKey(podName, namespace)] = &podAllocatedInfo{
		podUID:          podUID,
		ipInstanceNames: ipInstanceNames,
	}

	for _, ipInstanceName := range ipInstanceNames {
		c.ipToPod[namespacedKey(ipInstanceName, namespace)] = podName
	}

	c.logger.V(1).Info("record cache", "pod name", podName, "pod UID", podUID,
		"ip instances", ipInstanceNames)
}

func (c *podIPCache) Release(ipInstanceName, namespace string) {
	c.Lock()
	defer c.Unlock()

	podName, exist := c.ipToPod[namespacedKey(ipInstanceName, namespace)]
	if !exist {
		c.logger.V(1).Info("skip deleting a no exist pod cache", "ip instance", ipInstanceName,
			"namespace", namespace)
		return
	}

	delete(c.ipToPod, namespacedKey(ipInstanceName, namespace))

	info, exist := c.podToIP[namespacedKey(podName, namespace)]
	if !exist {
		c.logger.V(1).Info("skip deleting a no exist ip instance cache", "ip instance", ipInstanceName,
			"namespace", namespace, "pod name", podName)
		return
	}

	for index, name := range info.ipInstanceNames {
		if name == ipInstanceName {
			info.ipInstanceNames = append(info.ipInstanceNames[:index], info.ipInstanceNames[index+1:]...)
			break
		}
	}

	if len(info.ipInstanceNames) == 0 {
		delete(c.podToIP, namespacedKey(podName, namespace))
	}

	c.logger.V(1).Info("delete cache", "ip instance", ipInstanceName,
		"namespace", namespace, "pod name", podName)
}

func (c *podIPCache) Get(podName, namespace string) (bool, types.UID, []string) {
	c.Lock()
	defer c.Unlock()

	info, exist := c.podToIP[namespacedKey(podName, namespace)]
	if !exist {
		return false, "", nil
	}

	return true, info.podUID, utils.DeepCopyStringSlice(info.ipInstanceNames)
}

func namespacedKey(name, namespace string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
