package remotecluster

import (
	"fmt"
	"reflect"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
)

// remote cluster is managed by admin, no need to full synchronize
func (c *Controller) reconcileRemoteCluster(clusterName string) error {
	klog.Infof("Reconciling Remote Cluster %v", clusterName)
	remoteCluster, err := c.remoteClusterLister.Get(clusterName)
	if err != nil {
		if k8serror.IsNotFound(err) {
			c.delRemoteCluster(clusterName)
			return nil
		}
		return err
	}
	return c.addOrUpdateRCMgr(remoteCluster)
}

func (c *Controller) filterRemoteCluster(obj interface{}) bool {
	_, ok := obj.(*networkingv1.RemoteCluster)
	return ok
}

func (c *Controller) addOrDelRemoteCluster(obj interface{}) {
	rc, _ := obj.(*networkingv1.RemoteCluster)
	c.enqueueRemoteCluster(rc.Name)
}

func (c *Controller) updateRemoteCluster(oldObj, newObj interface{}) {
	oldRC, _ := oldObj.(*networkingv1.RemoteCluster)
	newRC, _ := newObj.(*networkingv1.RemoteCluster)

	if oldRC.ResourceVersion == newRC.ResourceVersion ||
		oldRC.Generation == newRC.Generation {
		return
	}
	if !remoteClusterSpecChanged(&oldRC.Spec, &newRC.Spec) {
		return
	}
	c.enqueueRemoteCluster(newRC.ClusterName)
}

func (c *Controller) enqueueRemoteCluster(clusterName string) {
	c.remoteClusterQueue.Add(clusterName)
}

func (c *Controller) delRemoteCluster(clusterName string) {
	klog.Infof("deleting cluster=%v.", clusterName)
	c.rcMgrCache.Del(clusterName)
}

func (c *Controller) runRemoteClusterWorker() {
	for c.processNextRemoteCluster() {
	}
}

func (c *Controller) processNextRemoteCluster() bool {
	obj, shutdown := c.remoteClusterQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.remoteClusterQueue.Done(obj)
		var (
			key string
			ok  bool
		)
		if key, ok = obj.(string); !ok {
			c.remoteClusterQueue.Forget(obj)
			return nil
		}
		if err := c.reconcileRemoteCluster(key); err != nil {
			// TODO: use retry handler to
			// Put the item back on the workqueue to handle any transient errors
			c.remoteClusterQueue.AddRateLimited(key)
			return fmt.Errorf("[remote cluster] fail to sync '%v': %v, requeuing", key, err)
		}
		c.remoteClusterQueue.Forget(obj)
		klog.Infof("[remote cluster] succeed to sync '%v'", key)
		return nil
	}(obj)

	if err != nil {
		klog.Error(err)
	}

	return true
}

func remoteClusterSpecChanged(old, new *networkingv1.RemoteClusterSpec) bool {
	return !reflect.DeepEqual(old.ConnConfig, new.ConnConfig)
}
