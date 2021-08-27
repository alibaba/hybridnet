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

package ipam

import (
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils/transform"
)

func (c *Controller) filterIP(obj interface{}) bool {
	_, ok := obj.(*v1.IPInstance)
	return ok
}

func (c *Controller) addIP(obj interface{}) {
	i, ok := obj.(*v1.IPInstance)
	if !ok {
		return
	}

	c.enqueueIP(i)
}

func (c *Controller) updateIP(oldObj, newObj interface{}) {
	old, ok := oldObj.(*v1.IPInstance)
	if !ok {
		return
	}
	new, ok := newObj.(*v1.IPInstance)
	if !ok {
		return
	}

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	c.enqueueIP(new)
}

func (c *Controller) enqueueIP(obj interface{}) {
	var (
		key string
		err error
	)
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		return
	}
	c.ipQueue.Add(key)
}

func (c *Controller) reconcileIP(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Errorf("invalid key %v", key)
		return nil
	}

	ipInstance, err := c.ipLister.IPInstances(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if ipInstance.DeletionTimestamp != nil {
		return c.releaseIP(ipInstance)
	}

	// Nothing to do when updating
	return nil
}

func (c *Controller) releaseIP(ipInstance *v1.IPInstance) (err error) {
	if feature.DualStackEnabled() {
		ipFamily := ipamtypes.IPv4Only
		if ipInstance.Spec.Address.Version == v1.IPv6 {
			ipFamily = ipamtypes.IPv6Only
		}

		if err = c.dualStackIPAMManager.Release(ipFamily, ipInstance.Spec.Network, []string{
			ipInstance.Spec.Subnet,
		}, []string{
			toIPFormat(ipInstance.Name),
		}); err != nil {
			return err
		}
		if err = c.dualStackIPAMStroe.IPUnBind(ipInstance.Namespace, ipInstance.Name); err != nil {
			return err
		}
	} else {
		if err = c.ipamManager.Release(ipInstance.Spec.Network, ipInstance.Spec.Subnet, toIPFormat(ipInstance.Name)); err != nil {
			return err
		}
		if err = c.ipamStore.IPUnBind(ipInstance.Namespace, ipInstance.Name); err != nil {
			return err
		}
	}
	return
}

func (c *Controller) ipSetGetter(subnet string) (ipamtypes.IPSet, error) {
	selector := labels.SelectorFromSet(labels.Set{
		constants.LabelSubnet: subnet,
	})

	ipList, err := c.ipLister.List(selector)
	if err != nil {
		return nil, err
	}

	ipSet := ipamtypes.NewIPSet()

	for _, item := range ipList {
		ipSet.Add(toIPFormat(item.Name), transform.TransferIPInstanceForIPAM(item))
	}

	return ipSet, nil
}

func toIPFormat(name string) string {
	const IPv6SeparatorCount = 7
	if isIPv6 := strings.Count(name, "-") == IPv6SeparatorCount; isIPv6 {
		return net.ParseIP(strings.ReplaceAll(name, "-", ":")).String()
	}
	return strings.ReplaceAll(name, "-", ".")
}
