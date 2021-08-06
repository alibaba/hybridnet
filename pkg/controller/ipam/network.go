/*
  Copyright 2021 The Rama Authors.

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
	"encoding/json"
	"reflect"

	v1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/feature"
	ipamtypes "github.com/oecp/rama/pkg/ipam/types"
	"github.com/oecp/rama/pkg/utils/transform"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

func (c *Controller) filterNetwork(obj interface{}) bool {
	_, ok := obj.(*v1.Network)
	return ok
}

func (c *Controller) addNetwork(obj interface{}) {
	n, _ := obj.(*v1.Network)
	c.enqueueNetwork(n.Name)
}

func (c *Controller) updateNetwork(oldObj, newObj interface{}) {
	old, _ := oldObj.(*v1.Network)
	new, _ := newObj.(*v1.Network)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	if old.Generation == new.Generation {
		return
	}

	if !networkSpecChanged(&old.Spec, &new.Spec) {
		return
	}

	c.enqueueNetwork(new.Name)
}

func (c *Controller) delNetwork(obj interface{}) {
	n, _ := obj.(*v1.Network)
	c.enqueueNetwork(n.Name)
}

func (c *Controller) enqueueNetwork(networkName string) {
	c.networkQueue.Add(networkName)
	c.networkStatusQueue.Add(networkName)
}

func (c *Controller) enqueueNetworkStatus(networkName string) {
	c.networkStatusQueue.Add(networkName)
}

func (c *Controller) reconcileNetwork(networkName string) error {
	network, err := c.networkLister.Get(networkName)
	switch {
	case err != nil && errors.IsNotFound(err):
		c.ipamCache.RemoveNetworkCache(networkName)
	case err != nil:
		return err
	default:
		c.ipamCache.UpdateNetworkCache(network)
	}

	// for updating node condition
	c.enqueueAllNode()

	return c.refreshNetwork(networkName)
}

func (c *Controller) refreshNetwork(networkName string) error {
	if feature.DualStackEnabled() {
		return c.dualStackIPAMManager.Refresh([]string{networkName})
	}
	return c.ipamManager.Refresh([]string{networkName})
}

func (c *Controller) reconcileNetworkStatus(networkName string) error {
	if len(networkName) == 0 {
		return nil
	}

	network, err := c.networkLister.Get(networkName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	c.ipamCache.UpdateNetworkCache(network)

	nodes, err := c.getRelatedNodesOfNetwork(networkName)
	if err != nil {
		return err
	}
	subnets, err := c.getRelatedSubnetsOfNetwork(networkName)
	if err != nil {
		return err
	}

	if feature.DualStackEnabled() {
		return c.dualStackIPAMStroe.SyncNetworkStatus(networkName, string(nodes), string(subnets))
	}
	return c.ipamStore.SyncNetworkStatus(networkName, string(nodes), string(subnets))
}

func (c *Controller) getRelatedNodesOfNetwork(networkName string) ([]byte, error) {
	var relatedNodes []string
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if c.ipamCache.MatchNetworkByLabels(networkName, node.Labels) {
			relatedNodes = append(relatedNodes, node.Name)
		}
	}

	return json.Marshal(relatedNodes)
}

func (c *Controller) getRelatedSubnetsOfNetwork(networkName string) ([]byte, error) {
	var relatedSubnets []string
	subnets, err := c.subnetLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, subnet := range subnets {
		if subnet.Spec.Network == networkName {
			relatedSubnets = append(relatedSubnets, subnet.Name)
		}
	}

	return json.Marshal(relatedSubnets)
}

// This function only care about the changed fields
// which are related to IPAM
func networkSpecChanged(old, new *v1.NetworkSpec) bool {
	return !reflect.DeepEqual(old.NetID, new.NetID) || !reflect.DeepEqual(old.NodeSelector, new.NodeSelector)
}

func (c *Controller) networkGetter(name string) (*ipamtypes.Network, error) {
	network, err := c.networkLister.Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return transform.TransferNetworkForIPAM(network), nil
}
