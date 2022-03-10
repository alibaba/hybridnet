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

package containernetwork

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alibaba/hybridnet/pkg/constants"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/daemon/bgp"
	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
	"github.com/vishvananda/netlink"
)

func GenerateContainerVethPair(podNamespace, podName string) (string, string) {
	// A SHA1 is always 20 bytes long, and so is sufficient for generating the
	// veth name and mac addr.
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s.%s", podNamespace, podName)))

	return fmt.Sprintf("%s%s", constants.ContainerHostLinkPrefix, hex.EncodeToString(h.Sum(nil))[:11]), constants.ContainerNicName
}

func CheckIfContainerNetworkLink(linkName string) bool {
	// TODO: suffix "_h" and prefix "h_" is deprecated, need to be removed further
	return strings.HasSuffix(linkName, "_h") ||
		strings.HasPrefix(linkName, "h_") ||
		strings.HasPrefix(linkName, constants.ContainerHostLinkPrefix) ||
		strings.HasPrefix(linkName, "veth") ||
		strings.HasPrefix(linkName, "docker")
}

func ListLocalAddressExceptLink(exceptLinkName string) ([]netlink.Addr, error) {
	var addrList []netlink.Addr

	linkList, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to list link: %v", err)
	}

	for _, link := range linkList {
		linkName := link.Attrs().Name
		if linkName != exceptLinkName && !CheckIfContainerNetworkLink(linkName) {

			linkAddrList, err := daemonutils.ListAllAddress(link)
			if err != nil {
				return nil, fmt.Errorf("failed to link addr for link %v: %v", link.Attrs().Name, err)
			}

			addrList = append(addrList, linkAddrList...)
		}
	}

	return addrList, nil
}

func EnsureRpFilterConfigs(containerHostIf string) error {
	for _, key := range []string{"default", "all"} {
		sysctlPath := fmt.Sprintf(constants.RpFilterSysctl, key)
		if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
			return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
		}
	}

	sysctlPath := fmt.Sprintf(constants.RpFilterSysctl, containerHostIf)
	if err := daemonutils.SetSysctl(sysctlPath, 0); err != nil {
		return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
	}

	existInterfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("error get exist interfaces on system: %v", err)
	}

	for _, existIf := range existInterfaces {
		if CheckIfContainerNetworkLink(existIf.Name) {
			continue
		}

		sysctlPath := fmt.Sprintf(constants.RpFilterSysctl, existIf.Name)
		sysctlValue, err := daemonutils.GetSysctl(sysctlPath)
		if err != nil {
			return fmt.Errorf("error get: %s sysctl path: %v", sysctlPath, err)
		}
		if sysctlValue != 0 {
			if err = daemonutils.SetSysctl(sysctlPath, 0); err != nil {
				return fmt.Errorf("error set: %s sysctl path to 0, error: %v", sysctlPath, err)
			}
		}
	}

	return nil
}

func checkPodNetConfigReady(podIP net.IP, podCidr *net.IPNet, forwardNodeIfIndex int, family int,
	networkMode networkingv1.NetworkMode, bgpManager *bgp.Manager) error {

	backOffBase := 100 * time.Microsecond
	retries := 5

	for i := 0; i < retries; i++ {
		switch networkMode {
		case networkingv1.NetworkModeVxlan, networkingv1.NetworkModeVlan:
			neighExist, err := daemonutils.CheckPodNeighExist(podIP, forwardNodeIfIndex, family)
			if err != nil {
				return fmt.Errorf("failed to check pod ip %v neigh exist: %v", podIP, err)
			}

			ruleExist, _, err := daemonutils.CheckPodRuleExist(podCidr, family)
			if err != nil {
				return fmt.Errorf("failed to check cidr %v rule exist: %v", podCidr, err)
			}

			if neighExist && ruleExist {
				// ready
				return nil
			}

			if i == retries-1 {
				if !neighExist {
					return fmt.Errorf("proxy neigh for %v is not created, waiting for daemon to create it", podIP)
				}

				if !ruleExist {
					return fmt.Errorf("policy rule for %v is not created, waiting for daemon to create it", podCidr)
				}
			}
		case networkingv1.NetworkModeBGP:
			ruleExist, table, err := daemonutils.CheckPodRuleExist(podCidr, family)
			if err != nil {
				return fmt.Errorf("failed to check cidr %v rule and default route exist: %v", podCidr, err)
			}

			defaultRouteExist := false
			if ruleExist {
				defaultRouteExist, err = daemonutils.CheckDefaultRouteExist(table, family)
				if err != nil {
					return fmt.Errorf("failed to check cidr %v default route exist: %v", podCidr, err)
				}
			}

			bgpPathExist, err := bgpManager.CheckIfIPInfoPathAdded(podIP)
			if err != nil {
				return fmt.Errorf("failed to check bgp path for pod ip %v: %v", podIP.String(), err)
			}
			establishedPeerExists, err := bgpManager.CheckEstablishedRemotePeerExists()
			if err != nil {
				return fmt.Errorf("failed to check established peer eixst: %v", err)
			}

			if ruleExist && defaultRouteExist && bgpPathExist && establishedPeerExists {
				// ready
				return nil
			}

			if i == retries-1 {
				if !ruleExist {
					return fmt.Errorf("policy rule for %v is not created, waiting for daemon to create it", podCidr)
				}

				if !defaultRouteExist {
					return fmt.Errorf("default route for %v is not created, waiting for daemon to create it", podCidr)
				}

				if !bgpPathExist {
					return fmt.Errorf("bgp path for pod ip %v ist not added, waiting for daemon to add it", podIP)
				}

				if !establishedPeerExists {
					return fmt.Errorf("none of the remote bgp peers is established, no bgp pod will be running")
				}
			}
		default:
			// do nothing
			return nil
		}

		time.Sleep(backOffBase)
		backOffBase = backOffBase * 2
	}

	return nil
}
