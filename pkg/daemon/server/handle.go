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

package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	clientset "github.com/oecp/rama/pkg/client/clientset/versioned"
	ramalister "github.com/oecp/rama/pkg/client/listers/networking/v1"
	"github.com/oecp/rama/pkg/constants"
	daemonconfig "github.com/oecp/rama/pkg/daemon/config"
	"github.com/oecp/rama/pkg/daemon/containernetwork"
	"github.com/oecp/rama/pkg/daemon/controller"
	"github.com/oecp/rama/pkg/request"

	"github.com/emicklei/go-restful"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type cniDaemonHandler struct {
	config     *daemonconfig.Configuration
	KubeClient kubernetes.Interface
	RamaClient clientset.Interface

	ipInstanceLister ramalister.IPInstanceLister
	ipInstanceSynced cache.InformerSynced

	networkLister ramalister.NetworkLister
}

func createCniDaemonHandler(stopCh <-chan struct{}, config *daemonconfig.Configuration, ctrlRef *controller.Controller) (*cniDaemonHandler, error) {
	cdh := &cniDaemonHandler{
		KubeClient:       config.KubeClient,
		RamaClient:       config.RamaClient,
		config:           config,
		ipInstanceLister: ctrlRef.GetIPInstanceLister(),
		networkLister:    ctrlRef.GetNetworkLister(),
		ipInstanceSynced: ctrlRef.GetIPInstanceSynced(),
	}

	if ok := cache.WaitForCacheSync(stopCh, cdh.ipInstanceSynced); !ok {
		return nil, fmt.Errorf("failed to wait for ip instance & pod caches to sync")
	}

	return cdh, nil
}

func (cdh cniDaemonHandler) handleAdd(req *restful.Request, resp *restful.Response) {
	podRequest := request.PodRequest{}
	err := req.ReadEntity(&podRequest)
	if err != nil {
		errMsg := fmt.Errorf("parse add request failed: %v", err)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusBadRequest, request.PodResponse{Err: errMsg.Error()})
		return
	}
	klog.Infof("Add port request %v", podRequest)

	var macAddr string
	var netID *uint32
	var affectedIPInstances []*ramav1.IPInstance

	allocatedIPs := map[ramav1.IPVersion]*containernetwork.IPInfo{
		ramav1.IPv4: nil,
		ramav1.IPv6: nil,
	}

	var returnIPAddress []request.IPAddress

	backOffBase := 5 * time.Microsecond
	retries := 11

	for i := 0; i < retries; i++ {
		time.Sleep(backOffBase)
		backOffBase = backOffBase * 2

		pod, err := cdh.KubeClient.CoreV1().Pods(podRequest.PodNamespace).Get(context.TODO(), podRequest.PodName, metav1.GetOptions{})
		if err != nil {
			errMsg := fmt.Errorf("get pod %v/%v failed: %v", podRequest.PodName, podRequest.PodNamespace, err)
			klog.Error(errMsg)
			_ = resp.WriteHeaderAndEntity(http.StatusBadRequest, request.PodResponse{Err: errMsg.Error()})
			return
		}

		// wait for ip instance to be coupled
		annotation := pod.GetAnnotations()
		_, exist := annotation[constants.AnnotationIP]
		if exist {
			break
		} else if i == retries-1 {
			errMsg := fmt.Errorf("wait for pod %v/%v be coupled with ip failed: %v", podRequest.PodName, podRequest.PodNamespace, err)
			klog.Error(errMsg)
			_ = resp.WriteHeaderAndEntity(http.StatusBadRequest, request.PodResponse{Err: errMsg.Error()})
			return
		}
	}

	ipSelector := labels.SelectorFromSet(labels.Set{
		constants.LabelNode: cdh.config.NodeName,
		constants.LabelPod:  podRequest.PodName,
	})

	ipInstanceList, err := cdh.ipInstanceLister.IPInstances(podRequest.PodNamespace).List(ipSelector)
	if err != nil {
		errMsg := fmt.Errorf("list ip instance for pod %v failed: %v", cdh.config.NodeName, err)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusBadRequest, request.PodResponse{Err: errMsg.Error()})
		return
	}

	var networkName string
	for _, ipInstance := range ipInstanceList {
		// IPv4 and IPv6 ip will exist at the same time
		if ipInstance.Status.PodName == podRequest.PodName && ipInstance.Status.PodNamespace == podRequest.PodNamespace {

			if netID == nil && macAddr == "" {
				netID = ipInstance.Spec.Address.NetID
				macAddr = ipInstance.Spec.Address.MAC
			} else if (netID != ipInstance.Spec.Address.NetID &&
				(netID != nil && *netID != *ipInstance.Spec.Address.NetID)) ||
				macAddr != ipInstance.Spec.Address.MAC {

				errMsg := fmt.Errorf("mac and netId for all ip instances of pod %v/%v should be the same", podRequest.PodNamespace, podRequest.PodName)
				klog.Error(errMsg)
				_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
				return
			}

			containerIP, cidrNet, err := net.ParseCIDR(ipInstance.Spec.Address.IP)
			if err != nil {
				errMsg := fmt.Errorf("parse ip address %v to cidr failed: %v", ipInstance.Spec.Address.IP, err)
				klog.Error(errMsg)
				_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
				return
			}

			gatewayIP := net.ParseIP(ipInstance.Spec.Address.Gateway)
			if gatewayIP == nil {
				errMsg := fmt.Errorf("parse gateway %v for ip %v failed: %v", ipInstance.Spec.Address.Gateway, ipInstance.Spec.Address.IP, err)
				klog.Error(errMsg)
				_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
				return
			}

			ipVersion := ramav1.IPv4
			switch ipInstance.Spec.Address.Version {
			case ramav1.IPv4:
				if allocatedIPs[ramav1.IPv4] != nil {
					errMsg := fmt.Errorf("only one ipv4 address for each pod are supported, %v/%v", podRequest.PodNamespace, podRequest.PodName)
					klog.Error(errMsg)
					_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
					return
				}

				allocatedIPs[ramav1.IPv4] = &containernetwork.IPInfo{
					Addr: containerIP,
					Gw:   gatewayIP,
					Cidr: cidrNet,
				}
			case ramav1.IPv6:
				if allocatedIPs[ramav1.IPv6] != nil {
					errMsg := fmt.Errorf("only one ipv6 address for each pod are supported, %v/%v", podRequest.PodNamespace, podRequest.PodName)
					klog.Error(errMsg)
					_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
					return
				}

				allocatedIPs[ramav1.IPv6] = &containernetwork.IPInfo{
					Addr: containerIP,
					Gw:   gatewayIP,
					Cidr: cidrNet,
				}

				ipVersion = ramav1.IPv6
			default:
				errMsg := fmt.Errorf("unsupported ip version %v for pod %v/%v", ipInstance.Spec.Address.Version, podRequest.PodNamespace, podRequest.PodName)
				klog.Error(errMsg)
				_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
				return
			}

			currentNetworkName := ipInstance.Spec.Network
			if networkName == "" {
				networkName = currentNetworkName
			} else {
				if networkName != currentNetworkName {
					errMsg := fmt.Errorf("found different networks %v/%v for pod %v/%v", currentNetworkName, networkName, podRequest.PodNamespace, podRequest.PodName)
					klog.Error(errMsg)
					_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
					return
				}
			}

			returnIPAddress = append(returnIPAddress, request.IPAddress{
				IP:       ipInstance.Spec.Address.IP,
				Mac:      ipInstance.Spec.Address.MAC,
				Gateway:  ipInstance.Spec.Address.Gateway,
				Protocol: ipVersion,
			})

			affectedIPInstances = append(affectedIPInstances, ipInstance)
		}
	}

	// check valid ip information second time
	if macAddr == "" || netID == nil {
		errMsg := fmt.Errorf("no available ip for pod %s/%s", podRequest.PodNamespace, podRequest.PodName)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
		return
	}

	network, err := cdh.networkLister.Get(networkName)
	if err != nil {
		errMsg := fmt.Errorf("cannot get network %v", networkName)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
		return
	}

	klog.Infof("Create container, mac %s, net id %d", macAddr, *netID)
	hostInterface, err := cdh.configureNic(podRequest.PodName, podRequest.PodNamespace, podRequest.NetNs, podRequest.ContainerID,
		macAddr, netID, allocatedIPs, ramav1.GetNetworkType(network))
	if err != nil {
		errMsg := fmt.Errorf("configure nic failed: %v", err)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
		return
	}

	// update IPInstance crd status
	for _, ip := range affectedIPInstances {
		newIPInstance := ip.DeepCopy()
		if newIPInstance == nil {
			errMsg := fmt.Errorf("failed to deepCopy IPInstance crd, no available for %s, %v", podRequest.PodName, err)
			klog.Error(errMsg)
			_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
			return
		}

		newIPInstance.Status.SandboxID = podRequest.ContainerID
		_, err = cdh.RamaClient.NetworkingV1().IPInstances(newIPInstance.Namespace).UpdateStatus(context.TODO(), newIPInstance, metav1.UpdateOptions{})
		if err != nil {
			errMsg := fmt.Errorf("failed to update IPInstance crd for %s, %v", newIPInstance.Name, err)
			klog.Error(errMsg)
			_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
			return
		}
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, request.PodResponse{
		IPAddress:     returnIPAddress,
		HostInterface: hostInterface,
	})
}

func (cdh cniDaemonHandler) handleDel(req *restful.Request, resp *restful.Response) {
	podRequest := request.PodRequest{}
	err := req.ReadEntity(&podRequest)
	if err != nil {
		errMsg := fmt.Errorf("parse del request failed: %v", err)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusBadRequest, request.PodResponse{Err: errMsg.Error()})
		return
	}

	klog.Infof("delete port request %v", podRequest)
	err = cdh.deleteNic(podRequest.NetNs)
	if err != nil {
		errMsg := fmt.Errorf("del container nic for %s failed: %v", fmt.Sprintf("%s.%s", podRequest.PodName, podRequest.PodNamespace), err)
		klog.Error(errMsg)
		_ = resp.WriteHeaderAndEntity(http.StatusInternalServerError, request.PodResponse{Err: errMsg.Error()})
		return
	}
	resp.WriteHeader(http.StatusNoContent)
}
