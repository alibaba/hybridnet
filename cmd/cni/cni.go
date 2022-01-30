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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"

	"github.com/vishvananda/netlink"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"

	"github.com/alibaba/hybridnet/pkg/request"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, "hybridnet cni")
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func cmdAdd(args *skel.CmdArgs) error {
	var err error

	netConf, cniVersion, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	podName, err := parseValueFromArgs("K8S_POD_NAME", args.Args)
	if err != nil {
		return err
	}

	podNamespace, err := parseValueFromArgs("K8S_POD_NAMESPACE", args.Args)
	if err != nil {
		return err
	}

	client := request.NewCniDaemonClient(netConf.ServerSocket)

	response, err := client.Add(request.PodRequest{
		PodName:      podName,
		PodNamespace: podNamespace,
		ContainerID:  args.ContainerID,
		NetNs:        args.Netns})
	if err != nil {
		return err
	}

	result, err := generateCNIResult(cniVersion, response)
	if err != nil {
		return fmt.Errorf("generate cni result failed: %v", err)
	}

	return types.PrintResult(result, cniVersion)
}

func generateCNIResult(cniVersion string, cniResponse *request.PodResponse) (*current.Result, error) {
	result := &current.Result{CNIVersion: cniVersion}
	result.IPs = []*current.IPConfig{}
	result.Routes = []*types.Route{}

	for _, address := range cniResponse.IPAddress {
		ipAddr, mask, _ := net.ParseCIDR(address.IP)
		ip := current.IPConfig{}
		route := types.Route{}

		switch address.Protocol {
		case networkingv1.IPv4:
			ip = current.IPConfig{
				Version: "4",
				Address: net.IPNet{IP: ipAddr.To4(), Mask: mask.Mask},
				Gateway: net.ParseIP(address.Gateway),
			}

			route = types.Route{
				Dst: net.IPNet{IP: net.ParseIP("0.0.0.0").To4(), Mask: net.CIDRMask(0, 32)},
				GW:  net.ParseIP(address.Gateway),
			}
		case networkingv1.IPv6:
			ip = current.IPConfig{
				Version: "6",
				Address: net.IPNet{IP: ipAddr.To16(), Mask: mask.Mask},
				Gateway: net.ParseIP(address.Gateway),
			}

			route = types.Route{
				Dst: net.IPNet{IP: net.ParseIP("::").To16(), Mask: net.CIDRMask(0, 128)},
				GW:  net.ParseIP(address.Gateway),
			}
		}

		result.IPs = append(result.IPs, &ip)
		result.Routes = append(result.Routes, &route)
	}

	// for chained cni plugins
	hostVeth, err := netlink.LinkByName(cniResponse.HostInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup host veth %q: %v", cniResponse.HostInterface, err)
	}

	hostIface := &current.Interface{}
	hostIface.Name = hostVeth.Attrs().Name
	hostIface.Mac = hostVeth.Attrs().HardwareAddr.String()

	result.Interfaces = []*current.Interface{hostIface}

	return result, nil
}

func cmdDel(args *skel.CmdArgs) error {
	netConf, _, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	client := request.NewCniDaemonClient(netConf.ServerSocket)
	podName, err := parseValueFromArgs("K8S_POD_NAME", args.Args)
	if err != nil {
		return err
	}
	podNamespace, err := parseValueFromArgs("K8S_POD_NAMESPACE", args.Args)
	if err != nil {
		return err
	}

	return client.Del(request.PodRequest{
		PodName:      podName,
		PodNamespace: podNamespace,
		ContainerID:  args.ContainerID,
		NetNs:        args.Netns})
}

type netConf struct {
	types.NetConf
	ServerSocket string `json:"server_socket"`
}

func loadNetConf(bytes []byte) (*netConf, string, error) {
	n := &netConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, "", fmt.Errorf("failed to load netconf: %v", err)
	}
	if n.ServerSocket == "" {
		return nil, "", fmt.Errorf("server_socket is required in cni.conf")
	}
	return n, n.CNIVersion, nil
}

func parseValueFromArgs(key, argString string) (string, error) {
	if argString == "" {
		return "", errors.New("CNI_ARGS is required")
	}
	args := strings.Split(argString, ";")
	for _, arg := range args {
		if strings.HasPrefix(arg, fmt.Sprintf("%s=", key)) {
			podName := strings.TrimPrefix(arg, fmt.Sprintf("%s=", key))
			if len(podName) > 0 {
				return podName, nil
			}
		}
	}
	return "", fmt.Errorf("%s is required in CNI_ARGS", key)
}
