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
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/alibaba/hybridnet/pkg/constants"

	"github.com/alibaba/hybridnet/pkg/controllers/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodIPCache interface {
	Record(podUID types.UID, podName, namespace string, ipInstanceNames []string)
	Release(ipInstanceName, namespace string) error
	Get(podName, namespace string) (bool, types.UID, []string)
}

type podAllocatedInfo struct {
	podUID          types.UID
	ipInstanceNames []string
}

type podIPCache struct {
	*sync.RWMutex

	// use "name/namespace" of ip instance as key
	podToIP map[string]*podAllocatedInfo

	// use "name/namespace" of ip instance as key and "name" of pod as value
	ipToPod map[string]string
}

func NewPodIPCache(c client.Reader) (PodIPCache, error) {
	cache := &podIPCache{
		podToIP: map[string]*podAllocatedInfo{},
		ipToPod: map[string]string{},
		RWMutex: &sync.RWMutex{},
	}

	ipList, err := utils.ListIPInstances(c)
	if err != nil {
		return nil, err
	}

	for _, ip := range ipList.Items {
		podName := ip.GetLabels()[constants.LabelPod]
		if len(podName) != 0 {
			var podUID types.UID
			pod, err := utils.GetPod(c, podName, ip.Namespace)
			if err != nil {
				if err = client.IgnoreNotFound(err); err != nil {
					return nil, fmt.Errorf("unable to get Pod %v for IPInstance %v: %v", podName, ip.Name, err)
				}
			}

			if pod != nil {
				podUID = pod.UID
			}

			var recordedIPInstances []string
			if cache.podToIP[namespacedKey(podName, ip.Namespace)] == nil {
				recordedIPInstances = nil
			} else {
				recordedIPInstances = cache.podToIP[namespacedKey(podName, ip.Namespace)].ipInstanceNames
			}

			// this is different from a normal Record action
			cache.podToIP[namespacedKey(podName, ip.Namespace)] = &podAllocatedInfo{
				podUID:          podUID,
				ipInstanceNames: append(recordedIPInstances, ip.Name),
			}

			cache.ipToPod[namespacedKey(ip.Name, ip.Namespace)] = podName
		}
	}

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
}

func (c *podIPCache) Release(ipInstanceName, namespace string) error {
	c.Lock()
	defer c.Unlock()

	podName, exist := c.ipToPod[namespacedKey(ipInstanceName, namespace)]
	if !exist {
		return fmt.Errorf("ipToPod record not exist for ip instance %v", ipInstanceName)
	}

	delete(c.ipToPod, namespacedKey(ipInstanceName, namespace))

	info := c.podToIP[namespacedKey(podName, namespace)]
	for index, name := range info.ipInstanceNames {
		if name == ipInstanceName {
			info.ipInstanceNames = append(info.ipInstanceNames[:index], info.ipInstanceNames[index+1:]...)
			break
		}
	}

	if len(info.ipInstanceNames) == 0 {
		delete(c.podToIP, namespacedKey(podName, namespace))
	}

	return nil
}

func (c *podIPCache) Get(podName, namespace string) (bool, types.UID, []string) {
	c.Lock()
	defer c.Unlock()

	info, exist := c.podToIP[namespacedKey(podName, namespace)]
	if !exist {
		return false, "", nil
	}

	return true, info.podUID, info.ipInstanceNames
}

func namespacedKey(name, namespace string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
