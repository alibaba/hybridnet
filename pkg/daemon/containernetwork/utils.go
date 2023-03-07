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
	"time"

	"github.com/go-ping/ping"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/daemon/bgp"
	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
)

func GenerateContainerVethPair(podNamespace, podName string) (string, string) {
	// A SHA1 is always 20 bytes long, and so is sufficient for generating the
	// veth name and mac addr.
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s.%s", podNamespace, podName)))

	return fmt.Sprintf("%s%s", constants.ContainerHostLinkPrefix, hex.EncodeToString(h.Sum(nil))[:11]), constants.ContainerNicName
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
		case networkingv1.NetworkModeBGP, networkingv1.NetworkModeGlobalBGP:
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
			establishedPeerExists, err := bgpManager.CheckRemotePeersEstablished()
			if err != nil {
				return fmt.Errorf("failed to check remote peers established: %v", err)
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
					return fmt.Errorf("bgp path for pod ip %v is not added, waiting for daemon to add it", podIP)
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

func CheckReachabilityFromHost(target net.IP, family int) error {
	addressList, err := daemonutils.ListGlobalUnicastAddress(nil, family)
	if err != nil {
		return fmt.Errorf("failed to list global unicase addresses: %v", err)
	}

	const retries = 5
	interval := time.Second

	if len(addressList) != 0 {
		pinger, err := ping.NewPinger(target.String())
		if err != nil {
			return fmt.Errorf("failed to init pinger: %v", err)
		}

		pinger.SetPrivileged(true)
		pinger.Count = retries
		pinger.Timeout = retries * interval
		pinger.Interval = interval

		var success bool
		pinger.OnRecv = func(p *ping.Packet) {
			success = true
			pinger.Stop()
		}

		if err = pinger.Run(); err != nil {
			return fmt.Errorf("failed to run pinger: %v", err)
		}

		if !success {
			return fmt.Errorf("pod network not ready after %d ping %s", retries, target)
		}
	}

	return nil
}
