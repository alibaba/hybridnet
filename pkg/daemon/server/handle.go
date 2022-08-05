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

package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/emicklei/go-restful"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/bgp"
	daemonconfig "github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/controller"
	"github.com/alibaba/hybridnet/pkg/daemon/utils"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/request"
	globalutils "github.com/alibaba/hybridnet/pkg/utils"
	webhookutils "github.com/alibaba/hybridnet/pkg/webhook/utils"
)

type cniDaemonHandler struct {
	config       *daemonconfig.Configuration
	mgrClient    client.Client
	mgrAPIReader client.Reader
	bgpManager   *bgp.Manager

	logger logr.Logger
}

func createCniDaemonHandler(ctx context.Context, config *daemonconfig.Configuration,
	ctrlRef *controller.CtrlHub, logger logr.Logger) (*cniDaemonHandler, error) {
	cdh := &cniDaemonHandler{
		config:       config,
		mgrClient:    ctrlRef.GetMgrClient(),
		mgrAPIReader: ctrlRef.GetMgrAPIReader(),
		bgpManager:   ctrlRef.GetBGPManager(),
		logger:       logger,
	}

	if ok := ctrlRef.CacheSynced(ctx); !ok {
		return nil, fmt.Errorf("failed to wait for ip instance & pod caches to sync")
	}

	return cdh, nil
}

func (cdh *cniDaemonHandler) handleAdd(req *restful.Request, resp *restful.Response) {
	podRequest := request.PodRequest{}
	err := req.ReadEntity(&podRequest)
	if err != nil {
		errMsg := fmt.Errorf("failed to parse add request: %v", err)
		cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
		return
	}
	cdh.logger.V(5).Info("handle add request", "content", podRequest)

	var macAddr string
	var netID *int32
	var affectedIPInstances []*networkingv1.IPInstance

	allocatedIPs := map[networkingv1.IPVersion]*utils.IPInfo{
		networkingv1.IPv4: nil,
		networkingv1.IPv6: nil,
	}

	var returnIPAddress []request.IPAddress
	var ipInstanceList []*networkingv1.IPInstance

	pod := &corev1.Pod{}
	if err := cdh.mgrAPIReader.Get(context.TODO(), types.NamespacedName{
		Name:      podRequest.PodName,
		Namespace: podRequest.PodNamespace,
	}, pod); err != nil {
		errMsg := fmt.Errorf("failed to get pod %v/%v: %v", podRequest.PodName, podRequest.PodNamespace, err)
		cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
		return
	}

	backOffBase := 5 * time.Microsecond
	retries := 11
	ipFamily := ipamtypes.ParseIPFamilyFromString(pod.Annotations[constants.AnnotationIPFamily])
	handledByWebhook := globalutils.ParseBoolOrDefault(pod.Annotations[constants.AnnotationHandledByWebhook], false)

	if !handledByWebhook {
		_, _, _, ipFamilyStr, err := webhookutils.ParseNetworkConfigOfPodByPriority(context.TODO(), cdh.mgrAPIReader, pod)
		if err != nil {
			errMsg := fmt.Errorf("failed to parse network config of pod %v: %v", pod.Name, err)
			cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
			return
		}
		ipFamily = ipamtypes.ParseIPFamilyFromString(ipFamilyStr)
	}

	for i := 0; i < retries; i++ {
		time.Sleep(backOffBase)
		backOffBase = backOffBase * 2

		if ipInstanceList, err = cdh.listAvailableIPInstanceOfPod(string(pod.GetUID()), podRequest.PodNamespace); err != nil {
			errMsg := fmt.Errorf("failed to list ip instances for pod %v/%v: %v",
				podRequest.PodName, podRequest.PodNamespace, err)
			cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
			return
		}

		var expectIPNumber int
		switch ipFamily {
		case ipamtypes.IPv4, ipamtypes.IPv6:
			expectIPNumber = 1
		case ipamtypes.DualStack:
			expectIPNumber = 2
		default:
			errMsg := fmt.Errorf("invalid ip family %v for pod %v/%v",
				ipFamily, podRequest.PodName, podRequest.PodNamespace)
			cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
			return
		}

		if len(ipInstanceList) == expectIPNumber {
			break
		} else if i == retries-1 {
			errMsg := fmt.Errorf("failed to wait for pod %v/%v to be coupled with ip, expect %v and get %v",
				podRequest.PodName, podRequest.PodNamespace, expectIPNumber, len(ipInstanceList))
			cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
			return
		}
	}

	var networkName string
	for _, ipInstance := range ipInstanceList {
		// IPv4 and IPv6 ip will exist at the same time
		if netID == nil && macAddr == "" {
			netID = ipInstance.Spec.Address.NetID
			macAddr = ipInstance.Spec.Address.MAC
		} else if (netID != ipInstance.Spec.Address.NetID &&
			(netID != nil && *netID != *ipInstance.Spec.Address.NetID)) ||
			macAddr != ipInstance.Spec.Address.MAC {

			errMsg := fmt.Errorf("mac and netId for all ip instances of pod %v/%v should be the same", podRequest.PodNamespace, podRequest.PodName)
			cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
			return
		}

		containerIP, cidrNet, err := net.ParseCIDR(ipInstance.Spec.Address.IP)
		if err != nil {
			errMsg := fmt.Errorf("failed to parse ip address %v to cidr: %v", ipInstance.Spec.Address.IP, err)
			cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
			return
		}

		gatewayIP := net.ParseIP(ipInstance.Spec.Address.Gateway)

		ipVersion := networkingv1.IPv4
		switch ipInstance.Spec.Address.Version {
		case networkingv1.IPv4:
			if allocatedIPs[networkingv1.IPv4] != nil {
				errMsg := fmt.Errorf("only one ipv4 address for each pod are supported, %v/%v", podRequest.PodNamespace, podRequest.PodName)
				cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
				return
			}

			allocatedIPs[networkingv1.IPv4] = &utils.IPInfo{
				Addr: containerIP,
				Gw:   gatewayIP,
				Cidr: cidrNet,
			}
		case networkingv1.IPv6:
			if allocatedIPs[networkingv1.IPv6] != nil {
				errMsg := fmt.Errorf("only one ipv6 address for each pod are supported, %v/%v", podRequest.PodNamespace, podRequest.PodName)
				cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
				return
			}

			allocatedIPs[networkingv1.IPv6] = &utils.IPInfo{
				Addr: containerIP,
				Gw:   gatewayIP,
				Cidr: cidrNet,
			}

			ipVersion = networkingv1.IPv6
		default:
			errMsg := fmt.Errorf("unsupported ip version %v for pod %v/%v", ipInstance.Spec.Address.Version, podRequest.PodNamespace, podRequest.PodName)
			cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
			return
		}

		currentNetworkName := ipInstance.Spec.Network
		if networkName == "" {
			networkName = currentNetworkName
		} else {
			if networkName != currentNetworkName {
				errMsg := fmt.Errorf("found different networks %v/%v for pod %v/%v", currentNetworkName, networkName, podRequest.PodNamespace, podRequest.PodName)
				cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
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

	// check valid ip information second time
	if macAddr == "" || len(allocatedIPs) == 0 {
		errMsg := fmt.Errorf("no available ip for pod %s/%s", podRequest.PodNamespace, podRequest.PodName)
		cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
		return
	}

	network := &networkingv1.Network{}
	if err := cdh.mgrClient.Get(context.TODO(), types.NamespacedName{Name: networkName}, network); err != nil {
		errMsg := fmt.Errorf("cannot get network %v", networkName)
		cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
		return
	}

	cdh.logger.Info("Create container",
		"podName", podRequest.PodName,
		"podNamespace", podRequest.PodNamespace,
		"ipAddr", printAllocatedIPs(allocatedIPs),
		"macAddr", macAddr)
	hostInterface, err := cdh.configureNic(podRequest.PodName, podRequest.PodNamespace, podRequest.NetNs, podRequest.ContainerID,
		macAddr, netID, allocatedIPs, networkingv1.GetNetworkMode(network))
	if err != nil {
		errMsg := fmt.Errorf("failed to configure nic: %v", err)
		cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
		return
	}
	cdh.logger.Info("Container network created",
		"podName", podRequest.PodName,
		"podNamespace", podRequest.PodNamespace,
		"ipAddr", printAllocatedIPs(allocatedIPs),
		"macAddr", macAddr)

	// update IPInstance crd status
	for _, ip := range affectedIPInstances {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var updateTimestamp string
			updateTimestamp, err = metav1.Now().MarshalQueryParameter()
			if err != nil {
				return fmt.Errorf("failed to generate update timestamp: %v", err)
			}

			return cdh.mgrClient.Status().Patch(context.TODO(), ip,
				client.RawPatch(types.MergePatchType,
					[]byte(fmt.Sprintf(`{"status":{"sandboxID":%q,"nodeName":%q,"podNamespace":%q,"podName":%q,"phase":null,"updateTimestamp":%q}}`,
						podRequest.ContainerID, cdh.config.NodeName, podRequest.PodNamespace, podRequest.PodName, updateTimestamp))))
		}); err != nil {
			errMsg := fmt.Errorf("failed to update IPInstance crd for %s, %v", ip.Name, err)
			cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
			return
		}
	}

	_ = resp.WriteHeaderAndEntity(http.StatusOK, request.PodResponse{
		IPAddress:     returnIPAddress,
		HostInterface: hostInterface,
	})
}

func (cdh *cniDaemonHandler) handleDel(req *restful.Request, resp *restful.Response) {
	podRequest := request.PodRequest{}
	err := req.ReadEntity(&podRequest)
	if err != nil {
		errMsg := fmt.Errorf("failed to parse del request: %v", err)
		cdh.errorWrapper(errMsg, http.StatusBadRequest, resp)
		return
	}

	cdh.logger.Info("Delete container",
		"podName", podRequest.PodName,
		"podNamespace", podRequest.PodNamespace,
	)

	cdh.logger.V(5).Info("handle del request", "content", podRequest)

	err = cdh.deleteNic(podRequest.NetNs)
	if err != nil {
		errMsg := fmt.Errorf("failed to del container nic for %s: %v",
			fmt.Sprintf("%s.%s", podRequest.PodName, podRequest.PodNamespace), err)
		cdh.errorWrapper(errMsg, http.StatusInternalServerError, resp)
		return
	}

	cdh.logger.Info("Container deleted",
		"podName", podRequest.PodName,
		"podNamespace", podRequest.PodNamespace,
	)

	resp.WriteHeader(http.StatusNoContent)
}

func (cdh *cniDaemonHandler) errorWrapper(err error, status int, resp *restful.Response) {
	cdh.logger.Error(err, "handler error")
	_ = resp.WriteHeaderAndEntity(status, request.PodResponse{
		Err: err.Error(),
	})
}

func (cdh *cniDaemonHandler) listAvailableIPInstanceOfPod(podUID, podNamespace string) ([]*networkingv1.IPInstance, error) {
	ipInstanceList := &networkingv1.IPInstanceList{}
	if err := cdh.mgrClient.List(context.TODO(), ipInstanceList, client.InNamespace(podNamespace), client.MatchingLabels{
		constants.LabelNode:   cdh.config.NodeName,
		constants.LabelPodUID: podUID,
	}); err != nil {
		return nil, err
	}

	var availableIPInstances []*networkingv1.IPInstance
	for index := range ipInstanceList.Items {
		if ipInstanceList.Items[index].DeletionTimestamp.IsZero() {
			availableIPInstances = append(availableIPInstances, &ipInstanceList.Items[index])
		}
	}

	return availableIPInstances, nil
}

func printAllocatedIPs(allocatedIPs map[networkingv1.IPVersion]*utils.IPInfo) string {
	ipAddressString := ""
	if allocatedIPs[networkingv1.IPv4] != nil && allocatedIPs[networkingv1.IPv4].Addr != nil {
		ipAddressString = ipAddressString + allocatedIPs[networkingv1.IPv4].Addr.String()
	}

	if allocatedIPs[networkingv1.IPv6] != nil && allocatedIPs[networkingv1.IPv6].Addr != nil {
		if ipAddressString != "" {
			ipAddressString = ipAddressString + "/"
		}
		ipAddressString = ipAddressString + allocatedIPs[networkingv1.IPv6].Addr.String()
	}

	return ipAddressString
}
