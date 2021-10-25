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
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
)

func (c *Controller) filterNode(obj interface{}) bool {
	_, ok := obj.(*corev1.Node)
	return ok
}

func (c *Controller) addNode(obj interface{}) {
	n, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	if network := c.ipamCache.SelectNetworkByLabels(n.Labels); len(network) > 0 {
		c.enqueueNetworkStatus(network)
	}
	c.enqueueNode(n.Name)
}

func (c *Controller) updateNode(oldObj, newObj interface{}) {
	old, ok := oldObj.(*corev1.Node)
	if !ok {
		return
	}
	new, ok := newObj.(*corev1.Node)
	if !ok {
		return
	}
	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	if reflect.DeepEqual(old.Labels, new.Labels) {
		return
	}

	oldNetwork := c.ipamCache.SelectNetworkByLabels(old.Labels)
	newNetwork := c.ipamCache.SelectNetworkByLabels(new.Labels)

	if oldNetwork == newNetwork {
		return
	}
	if len(oldNetwork) > 0 {
		c.enqueueNetworkStatus(oldNetwork)
	}
	if len(newNetwork) > 0 {
		c.enqueueNetworkStatus(newNetwork)
	}
	c.enqueueNode(new.Name)
}

func (c *Controller) delNode(obj interface{}) {
	n, ok := obj.(*corev1.Node)
	if !ok {
		return
	}

	if network := c.ipamCache.SelectNetworkByLabels(n.Labels); len(network) > 0 {
		c.enqueueNetworkStatus(network)
	}
}

func (c *Controller) enqueueNode(name string) {
	c.nodeQueue.Add(name)
}

func (c *Controller) enqueueAllNode() {
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		return
	}

	for _, node := range nodes {
		c.enqueueNode(node.Name)
	}
}

func (c *Controller) reconcileNode(name string) error {
	node, err := c.nodeLister.Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if feature.DualStackEnabled() {
		if err = c.updateNodeQuotaLabels(node); err != nil {
			return err
		}
	}

	return c.updateNodeNetworkAttachment(node)
}

// updateNodeQuotaLabels only works on dual stack mode
func (c *Controller) updateNodeQuotaLabels(node *corev1.Node) error {
	var networkName string
	if networkName = c.ipamCache.SelectNetworkByLabels(node.Labels); len(networkName) > 0 {
		var v4Quota, v6Quota, dualStackQuota = constants.QuotaEmpty, constants.QuotaEmpty, constants.QuotaEmpty
		var networkUsages = c.ipamCache.GetNetworkUsages(networkName)
		if networkUsages[0] != nil && networkUsages[0].Available > 0 {
			v4Quota = constants.QuotaNonEmpty
		}
		if networkUsages[1] != nil && networkUsages[1].Available > 0 {
			v6Quota = constants.QuotaNonEmpty
		}
		if networkUsages[2] != nil && networkUsages[2].Available > 0 {
			dualStackQuota = constants.QuotaNonEmpty
		}

		return c.setNodeQuotaLabels(node.Name, v4Quota, v6Quota, dualStackQuota)
	}

	return c.setNodeQuotaLabels(node.Name, constants.QuotaEmpty, constants.QuotaEmpty, constants.QuotaEmpty)
}

func (c *Controller) updateNodeNetworkAttachment(node *corev1.Node) error {
	var underlayNetworkAttached = len(c.ipamCache.SelectNetworkByLabels(node.Labels)) > 0
	var overlayNetworkAttached = len(c.ipamCache.GetGlobalNetwork()) > 0

	var expectedTaints = node.Spec.DeepCopy().Taints
	if underlayNetworkAttached {
		expectedTaints = removeSpecificTaint(expectedTaints, constants.TaintUnderlayNetworkUnattached)
	} else {
		expectedTaints = ensureSpecificTaint(expectedTaints, constants.TaintUnderlayNetworkUnattached)
	}

	if overlayNetworkAttached {
		expectedTaints = removeSpecificTaint(expectedTaints, constants.TaintOverlayNetworkUnattached)
	} else {
		expectedTaints = ensureSpecificTaint(expectedTaints, constants.TaintOverlayNetworkUnattached)
	}

	if reflect.DeepEqual(node.Spec.Taints, expectedTaints) {
		return nil
	}

	return c.patchNodeTaints(node.Name, expectedTaints)
}

func (c *Controller) setNodeQuotaLabels(nodeName string, ipv4, ipv6, dualStack string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := c.kubeClientSet.CoreV1().Nodes().Patch(context.TODO(),
			nodeName,
			types.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"metadata":{"labels":{"%s":%q,"%s":%q,"%s":%q}}}`,
				constants.LabelIPv4AddressQuota,
				ipv4,
				constants.LabelIPv6AddressQuota,
				ipv6,
				constants.LabelDualStackAddressQuota,
				dualStack,
			)),
			metav1.PatchOptions{},
		)
		return err
	})
}

func (c *Controller) patchNodeTaints(nodeName string, taints []corev1.Taint) error {
	taintBytes, err := json.Marshal(taints)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := c.kubeClientSet.CoreV1().Nodes().Patch(context.TODO(),
			nodeName,
			types.MergePatchType,
			[]byte(fmt.Sprintf(
				`{"spec":{"taints":%q}}`,
				string(taintBytes),
			)),
			metav1.PatchOptions{},
		)
		return err
	})
}

func removeSpecificTaint(taints []corev1.Taint, taintKey string) []corev1.Taint {
	var idx = -1
	for i := range taints {
		if taints[i].Key == taintKey {
			idx = i
			break
		}
	}

	if idx < 0 {
		return taints
	}

	return append(taints[:idx], taints[idx+1:]...)
}

func ensureSpecificTaint(taints []corev1.Taint, taintKey string) []corev1.Taint {
	var idx = -1
	for i := range taints {
		if taints[i].Key == taintKey {
			idx = i
			break
		}
	}

	if idx < 0 {
		return append(taints, corev1.Taint{
			Key:    taintKey,
			Effect: corev1.TaintEffectNoSchedule,
		})
	}
	return taints
}
