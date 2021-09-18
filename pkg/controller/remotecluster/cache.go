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

package remotecluster

import (
	"sync"

	"github.com/oecp/rama/pkg/rcmanager"
	"k8s.io/klog"
)

type RcMgrCache struct {
	*sync.RWMutex
	rcMgrMap map[string]*rcmanager.Manager
}

func NewCache() *RcMgrCache {
	return &RcMgrCache{
		RWMutex:  new(sync.RWMutex),
		rcMgrMap: make(map[string]*rcmanager.Manager),
	}
}

func (c *RcMgrCache) Get(clusterName string) (manager *rcmanager.Manager, exists bool) {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	manager, exists = c.rcMgrMap[clusterName]
	return
}

func (c *RcMgrCache) Set(clusterName string, manager *rcmanager.Manager) {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()

	c.rcMgrMap[clusterName] = manager
}

func (c *RcMgrCache) Del(clusterName string) {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()

	if rc, exists := c.rcMgrMap[clusterName]; exists {
		klog.Infof("Delete cluster %v from cache", clusterName)
		delete(c.rcMgrMap, clusterName)
		rc.Close()
	}
}
