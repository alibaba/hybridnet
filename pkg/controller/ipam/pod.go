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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/feature"
	"github.com/alibaba/hybridnet/pkg/ipam/strategy"
	"github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/metrics"
	"github.com/alibaba/hybridnet/pkg/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

const (
	ReasonIPAllocationSucceed = "IPAllocationSucceed"
	ReasonIPAllocationFail    = "IPAllocationFail"
	ReasonIPReleaseSucceed    = "IPReleaseSucceed"
)

func (c *Controller) filterPod(obj interface{}) bool {
	p, ok := obj.(*v1.Pod)
	if !ok {
		return false
	}

	// ignore pod with host network
	if p.Spec.HostNetwork {
		return false
	}

	// only pod after scheduling and before IP allocating can be processed
	return len(p.Spec.NodeName) > 0 && !metav1.HasAnnotation(p.ObjectMeta, constants.AnnotationIP)
}

func (c *Controller) addPod(obj interface{}) {
	p, ok := obj.(*v1.Pod)
	if !ok {
		return
	}

	c.enqueuePod(p)
}

func (c *Controller) updatePod(oldObj, newObj interface{}) {
	old, ok := oldObj.(*v1.Pod)
	if !ok {
		return
	}
	new, ok := newObj.(*v1.Pod)
	if !ok {
		return
	}

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	c.enqueuePod(new)
}

func (c *Controller) enqueuePod(obj interface{}) {
	var (
		key string
		err error
	)
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		return
	}
	c.podQueue.Add(key)
}

func (c *Controller) reconcilePod(key string) (err error) {
	var (
		pod         *v1.Pod
		networkName string
	)

	defer func() {
		if err != nil && pod != nil {
			c.recorder.Event(pod, v1.EventTypeWarning, ReasonIPAllocationFail, err.Error())
		}
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Errorf("invalid key %v", key)
		return nil
	}

	pod, err = c.kubeClientSet.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("fail to get pod %s from client: %v", key, err)
	}

	// IP recycle rely on garbage collection, just ignore this
	if pod.DeletionTimestamp != nil {
		return nil
	}

	// To avoid IP duplicate allocation in high-frequent pod updates scenario because of
	// the fucking *delay* of informer
	if metav1.HasAnnotation(pod.ObjectMeta, constants.AnnotationIP) {
		return nil
	}

	networkName, err = c.selectNetwork(pod)
	if err != nil {
		return fmt.Errorf("fail to select network for pod %s: %v", key, err)
	}

	if strategy.OwnByStatefulWorkload(pod) {
		klog.Infof("strategic allocation for pod %s", key)
		return c.statefulAllocate(key, networkName, pod)
	}

	if strategy.OwnByStatelessWorkload(pod) {
		klog.Infof("non support strategic IP allocation for stateless workloads")
	}

	return c.allocate(key, networkName, pod)
}

func (c *Controller) statefulAllocate(key, networkName string, pod *v1.Pod) (err error) {
	var (
		preAssign     = len(pod.Annotations[constants.AnnotationIPPool]) > 0
		shouldObserve = true
		startTime     = time.Now()
		// reallocate means that ip should not be retained
		// 1. global retain and pod retain or unset, ip should be retained
		// 2. global retain and pod not retain, ip should be reallocated
		// 3. global not retain and pod not retain or unset, ip should be reallocated
		// 4. global not retain and pod retain, ip should be retained
		shouldReallocate = !utils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationIPRetain], strategy.DefaultIPRetain)
	)

	defer func() {
		if shouldObserve {
			metrics.IPAllocationPeriodSummary.
				WithLabelValues(metrics.IPStatefulAllocateType, strconv.FormatBool(err == nil)).
				Observe(float64(time.Since(startTime).Nanoseconds()))
		}
	}()

	if feature.DualStackEnabled() {
		var ipCandidates []string
		var ipFamilyMode = types.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])

		switch {
		case preAssign:
			ipPool := strings.Split(pod.Annotations[constants.AnnotationIPPool], ",")
			if idx := strategy.GetIndexFromName(pod.Name); idx < len(ipPool) {
				ipCandidates = strings.Split(ipPool[idx], "/")
				for i := range ipCandidates {
					ipCandidates[i] = utils.NormalizedIP(ipCandidates[i])
				}
			} else {
				err = fmt.Errorf("no available ip in ip-pool %s for pod %s", pod.Annotations[constants.AnnotationIPPool], key)
				return err
			}
		case shouldReallocate:
			var allocatedIPs []*types.IP
			if allocatedIPs, err = strategy.GetAllocatedIPsByPod(c.ipLister, pod); err != nil {
				return err
			}

			// reallocate means that the allocated ones should be recycled firstly
			if len(allocatedIPs) > 0 {
				if err = c.release(pod, allocatedIPs); err != nil {
					return err
				}
			}

			// reallocate
			return c.allocate(key, networkName, pod)
		default:
			if ipCandidates, err = strategy.GetIPsByPod(c.ipLister, pod); err != nil {
				return err
			}

			// when no valid ip found, it means that this is the first time of pod creation
			if len(ipCandidates) == 0 {
				// allocate has its own observation process, so just skip
				shouldObserve = false
				return c.allocate(key, networkName, pod)
			}
		}

		// forced assign for using reserved ips
		return c.multiAssign(networkName, pod, ipFamilyMode, ipCandidates, true)
	}

	var ipCandidate string

	switch {
	case preAssign:
		ipPool := strings.Split(pod.Annotations[constants.AnnotationIPPool], ",")
		if idx := strategy.GetIndexFromName(pod.Name); idx < len(ipPool) {
			ipCandidate = utils.NormalizedIP(ipPool[idx])
		}
		if len(ipCandidate) == 0 {
			err = fmt.Errorf("no available ip in ip-pool %s for pod %s", pod.Annotations[constants.AnnotationIPPool], key)
			return err
		}
	case shouldReallocate:
		var allocatedIPs []*types.IP
		if allocatedIPs, err = strategy.GetAllocatedIPsByPod(c.ipLister, pod); err != nil {
			return err
		}

		// reallocate means that the allocated ones should be recycled firstly
		if len(allocatedIPs) > 0 {
			if err = c.release(pod, allocatedIPs); err != nil {
				return err
			}
		}

		// reallocate
		return c.allocate(key, networkName, pod)
	default:
		ipCandidate, err = strategy.GetIPByPod(c.ipLister, pod)
		if err != nil {
			return err
		}
		// when no valid ip found, it means that this is the first time of pod creation
		if len(ipCandidate) == 0 {
			// allocate has its own observation process, so just skip
			shouldObserve = false
			return c.allocate(key, networkName, pod)
		}

	}

	// forced assign for using reserved ip
	return c.assign(networkName, pod, ipCandidate, true)
}

func (c *Controller) assign(networkName string, pod *v1.Pod, ipCandidate string, forced bool) (err error) {
	ip, err := c.ipamManager.Assign(networkName, "", pod.Name, pod.Namespace, ipCandidate, forced)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = c.ipamManager.Release(ip.Network, ip.Subnet, ip.Address.IP.String())
		}
	}()

	err = c.ipamStore.ReCouple(pod, ip)
	if err != nil {
		return fmt.Errorf("fail to force-couple ip with pod: %v", err)
	}

	c.recorder.Eventf(pod, v1.EventTypeNormal, ReasonIPAllocationSucceed, "assign IP %s successfully", ip.String())

	return nil
}

func (c *Controller) multiAssign(networkName string, pod *v1.Pod, ipFamily types.IPFamilyMode, ipCandidates []string, forced bool) (err error) {
	var IPs []*types.IP
	if IPs, err = c.dualStackIPAMManager.Assign(ipFamily, networkName, nil, ipCandidates, pod.Name, pod.Namespace, forced); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = c.dualStackIPAMManager.Release(ipFamily, networkName, squashIPSliceToSubnets(IPs), squashIPSliceToIPs(IPs))
		}
	}()

	if err = c.dualStackIPAMStroe.ReCouple(pod, IPs); err != nil {
		return fmt.Errorf("fail to force-couple ips %+v with pod: %v", IPs, err)
	}

	c.recorder.Eventf(pod, v1.EventTypeNormal, ReasonIPAllocationSucceed, "assign IPs %v successfully", squashIPSliceToIPs(IPs))
	return nil
}

func (c *Controller) allocate(key, networkName string, pod *v1.Pod) (err error) {
	var startTime = time.Now()
	defer func() {
		metrics.IPAllocationPeriodSummary.
			WithLabelValues(metrics.IPNormalAllocateType, strconv.FormatBool(err == nil)).
			Observe(float64(time.Since(startTime).Nanoseconds()))
	}()

	if feature.DualStackEnabled() {
		var (
			subnetNames  []string
			ips          []*types.IP
			ipFamilyMode = types.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])
		)
		if subnetNameStr := utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet], pod.Labels[constants.LabelSpecifiedSubnet]); len(subnetNameStr) > 0 {
			subnetNames = strings.Split(subnetNameStr, "/")
		}
		if ips, err = c.dualStackIPAMManager.Allocate(ipFamilyMode, networkName, subnetNames, pod.Name, pod.Namespace); err != nil {
			return fmt.Errorf("fail to allocate %s ip for pod %s: %v", ipFamilyMode, key, err)
		}
		defer func() {
			if err != nil {
				_ = c.dualStackIPAMManager.Release(ipFamilyMode, networkName, squashIPSliceToSubnets(ips), squashIPSliceToIPs(ips))
			}
		}()

		if err = c.dualStackIPAMStroe.Couple(pod, ips); err != nil {
			return fmt.Errorf("fail to couple IPs with pod: %v", err)
		}

		c.recorder.Eventf(pod, v1.EventTypeNormal, ReasonIPAllocationSucceed, "allocate IPs %v successfully", squashIPSliceToIPs(ips))
	} else {
		var (
			subnetName = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedSubnet], pod.Labels[constants.LabelSpecifiedSubnet])
			ip         *types.IP
		)
		if ip, err = c.ipamManager.Allocate(networkName, subnetName, pod.Name, pod.Namespace); err != nil {
			return fmt.Errorf("fail to allocate ip for pod %s: %v", key, err)
		}
		defer func() {
			if err != nil {
				_ = c.ipamManager.Release(ip.Network, ip.Subnet, ip.Address.IP.String())
			}
		}()

		if err = c.ipamStore.Couple(pod, ip); err != nil {
			return fmt.Errorf("fail to couple ip with pod: %v", err)
		}

		c.recorder.Eventf(pod, v1.EventTypeNormal, ReasonIPAllocationSucceed, "allocate IP %s successfully", ip.String())
	}
	return nil
}

func (c *Controller) release(pod *v1.Pod, allocatedIPs []*types.IP) (err error) {
	var recycleFunc func(namespace string, ip *types.IP) (err error)
	if feature.DualStackEnabled() {
		recycleFunc = c.dualStackIPAMStroe.IPRecycle
	} else {
		recycleFunc = c.ipamStore.IPRecycle
	}

	for _, ip := range allocatedIPs {
		if err = recycleFunc(pod.Namespace, ip); err != nil {
			return fmt.Errorf("fail to recycle ip %v for pod %s", ip, pod.Name)
		}
	}

	c.recorder.Eventf(pod, v1.EventTypeNormal, ReasonIPReleaseSucceed, "release IPs %v successfully", squashIPSliceToIPs(allocatedIPs))
	return nil
}

// selectNetwork will pick the hit network by pod, taking the priority as below
// 1. explicitly specify network in pod annotations/labels
// 2. parse network type from pod and select a corresponding network bind on node
func (c *Controller) selectNetwork(pod *v1.Pod) (string, error) {
	var specifiedNetwork string
	if specifiedNetwork = utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationSpecifiedNetwork], pod.Labels[constants.LabelSpecifiedNetwork]); len(specifiedNetwork) > 0 {
		return specifiedNetwork, nil
	}

	var networkType = types.ParseNetworkTypeFromString(utils.PickFirstNonEmptyString(pod.Annotations[constants.AnnotationNetworkType], pod.Labels[constants.LabelNetworkType]))
	switch networkType {
	case types.Underlay:
		return c.getNetworkFromNode(pod.Spec.NodeName, types.Underlay)
	case types.Overlay:
		networkCandidates := c.getNetworksByType(networkType)
		if len(networkCandidates) == 0 {
			return "", fmt.Errorf("required overlay network type but none found")
		}
		return networkCandidates[0], nil
	default:
		return "", fmt.Errorf("unknown network type %s", networkType)
	}
}

func (c *Controller) getNetworkFromNode(nodeName string, networkType types.NetworkType) (string, error) {
	node, err := c.nodeLister.Get(nodeName)
	if err != nil {
		return "", err
	}

	network := c.ipamCache.SelectNetworkByLabels(node.Labels)
	if len(network) == 0 {
		return "", fmt.Errorf("no network match node %s", nodeName)
	}

	return c.matchNetworkType(network, networkType)
}

// check network match specified network type
func (c *Controller) matchNetworkType(networkName string, networkType types.NetworkType) (string, error) {
	matched := (feature.DualStackEnabled() && c.dualStackIPAMManager.MatchNetworkType(networkName, networkType)) ||
		(!feature.DualStackEnabled() && c.ipamManager.MatchNetworkType(networkName, networkType))

	if matched {
		return networkName, nil
	}
	return "", fmt.Errorf("network %s does not match type %s", networkName, networkType)
}

func (c *Controller) getNetworksByType(networkType types.NetworkType) []string {
	if feature.DualStackEnabled() {
		return c.dualStackIPAMManager.GetNetworksByType(networkType)
	}
	return c.ipamManager.GetNetworksByType(networkType)
}

func squashIPSliceToIPs(ips []*types.IP) (ret []string) {
	for _, ip := range ips {
		ret = append(ret, ip.Address.IP.String())
	}
	return
}

func squashIPSliceToSubnets(ips []*types.IP) (ret []string) {
	for _, ip := range ips {
		ret = append(ret, ip.Subnet)
	}
	return
}
