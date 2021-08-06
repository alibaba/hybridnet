package remotecluster

import (
	"sync"

	"github.com/oecp/rama/pkg/rcmanager"
	"k8s.io/klog"
)

type Cache struct {
	mu       sync.RWMutex
	rcMgrMap map[string]*rcmanager.Manager
}

func (c *Cache) Get(clusterName string) (manager *rcmanager.Manager, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	manager, exists = c.rcMgrMap[clusterName]
	return
}

func (c *Cache) Set(clusterName string, manager *rcmanager.Manager) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.rcMgrMap[clusterName] = manager
}

func (c *Cache) Del(clusterName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rc, exists := c.rcMgrMap[clusterName]; exists {
		klog.Infof("Delete cluster %v from cache", clusterName)
		close(rc.StopCh)
		delete(c.rcMgrMap, clusterName)
	}
}
