package remotecluster

import (
	"sync"

	"github.com/oecp/rama/pkg/rcmanager"
	"k8s.io/klog"
)

type Cache struct {
	*sync.RWMutex
	rcMgrMap map[string]*rcmanager.Manager
}

func NewCache() *Cache {
	return &Cache{
		RWMutex:  new(sync.RWMutex),
		rcMgrMap: make(map[string]*rcmanager.Manager),
	}
}

func (c *Cache) Get(clusterName string) (manager *rcmanager.Manager, exists bool) {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	manager, exists = c.rcMgrMap[clusterName]
	return
}

func (c *Cache) Set(clusterName string, manager *rcmanager.Manager) {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()

	c.rcMgrMap[clusterName] = manager
}

func (c *Cache) Del(clusterName string) {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()
	if rc, exists := c.rcMgrMap[clusterName]; exists {
		klog.Infof("Delete cluster %v from cache", clusterName)
		rc.Close()
		delete(c.rcMgrMap, clusterName)
	}
}
