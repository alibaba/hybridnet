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

package config

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
	"github.com/alibaba/hybridnet/pkg/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/vishvananda/netlink"
)

const (
	DefaultHealthyServerBindAddress = ":11021"
	DefaultMetricsServerBindAddress = ":8091"
	DefaultBGPgRPCServerBindAddress = ":50051"

	DefaultVxlanUDPPort = 8472

	DefaultVlanCheckTimeout                     = 3 * time.Second
	DefaultIPtablesCheckDuration                = 5 * time.Second
	DefaultVxlanBaseReachableTime               = 5 * time.Second
	DefaultVxlanExpiredNeighCachesClearInterval = 1 * time.Hour

	DefaultNeighGCThresh1 = 1024
	DefaultNeighGCThresh2 = 2048
	DefaultNeighGCThresh3 = 4096

	DefaultLocalDirectTableNum     = 39999
	DefaultToOverlaySubnetTableNum = 40000
	DefaultOverlayMarkTableNum     = 40001

	DefaultIPv6RouteCacheMaxSize  = 524288
	DefaultIPv6RouteCacheGCThresh = 65536
)

// Configuration is the daemon conf
type Configuration struct {
	BindSocket string
	NodeName   string

	VlanMTU  int
	VxlanMTU int
	BGPMTU   int

	NodeVlanIfName  string
	NodeVxlanIfName string
	NodeBGPIfName   string

	ExtraNodeLocalVxlanIPCidrs []*net.IPNet

	HealthyServerAddress string
	MetricsServerAddress string
	BGPgRPCServerAddress string

	VxlanUDPPort int

	VlanCheckTimeout      time.Duration
	IptablesCheckDuration time.Duration

	VxlanBaseReachableTime               time.Duration
	VxlanExpiredNeighCachesClearInterval time.Duration
	VtepAddressCIDRs                     []*net.IPNet

	// Use fixed table num to mark "local-pod-direct rule"
	LocalDirectTableNum int

	// Use fixed table num to mark "to-overlay-pod-subnet rule"
	ToOverlaySubnetTableNum int

	// Use fixed table num to mark "overlay-mark-table rule"
	OverlayMarkTableNum int

	NeighGCThresh1 int
	NeighGCThresh2 int
	NeighGCThresh3 int

	IPv6RouteCacheMaxSize  int
	IPv6RouteCacheGCThresh int

	EnableVlanArpEnhancement     bool
	PatchCalicoPodIPsAnnotation  bool
	CheckPodConnectivityFromHost bool
}

// ParseFlags will parse cmd args then init kubeClient and configuration
func ParseFlags() (*Configuration, error) {
	var (
		argPreferInterfaces                     = pflag.String("prefer-interfaces", "", "[deprecated]The preferred vlan interfaces used to inter-host pod communication, default: the default route interface")
		argPreferVlanInterfaces                 = pflag.String("prefer-vlan-interfaces", "", "The preferred vlan interfaces used to inter-host pod communication, default: the default route interface")
		argPreferVxlanInterfaces                = pflag.String("prefer-vxlan-interfaces", "", "The preferred vxlan interfaces used to inter-host pod communication, default: the default route interface")
		argPreferBGPInterfaces                  = pflag.String("prefer-bgp-interfaces", "", "The preferred bgp interfaces used to inter-host pod communication, default: the default route interface")
		argBindSocket                           = pflag.String("bind-socket", "/var/run/hybridnet.sock", "The socket daemon bind to.")
		argHealthyServerAddress                 = pflag.String("health-probe-addr", DefaultHealthyServerBindAddress, "The address which daemon healthy server bind")
		argMetricsServerAddress                 = pflag.String("metrics-addr", DefaultMetricsServerBindAddress, "The address which daemon metrics server bind")
		argBGPgRPCServerAddress                 = pflag.String("bgp-grpc-server-addr", DefaultBGPgRPCServerBindAddress, "The address which daemon bgp grpc server bind, for using gobgp command to debug")
		argLocalDirectTableNum                  = pflag.Int("local-direct-table", DefaultLocalDirectTableNum, "The number of local-pod-direct route table")
		argIPtablesCheckDuration                = pflag.Duration("iptables-check-duration", DefaultIPtablesCheckDuration, "The time period for iptables manager to check iptables rules")
		argToOverlaySubnetTableNum              = pflag.Int("to-overlay-table", DefaultToOverlaySubnetTableNum, "The number of to-overlay-pod-subnet route table")
		argOverlayMarkTableNum                  = pflag.Int("overlay-mark-table", DefaultOverlayMarkTableNum, "The number of overlay-mark routing table")
		argVlanCheckTimeout                     = pflag.Duration("vlan-check-timeout", DefaultVlanCheckTimeout, "The timeout of vlan network environment check while pod creating")
		argVxlanUDPPort                         = pflag.Int("vxlan-udp-port", DefaultVxlanUDPPort, "The local udp port which vxlan tunnel use")
		argVxlanBaseReachableTime               = pflag.Duration("vxlan-base-reachable-time", DefaultVxlanBaseReachableTime, "The time for neigh caches of vxlan device to get STALE from REACHABLE")
		argVxlanExpiredNeighCachesClearInterval = pflag.Duration("vxlan-expired-neigh-caches-clear-interval", DefaultVxlanExpiredNeighCachesClearInterval, "The interval for daemon to clear STALE and FAILED neigh caches of vxlan device")
		argVtepAddressCIDRs                     = pflag.String("vtep-address-cidrs", "0.0.0.0/0,::/0", "The cidr list to select vtep address on each node, e.g., \\\"192.168.10.0/24,10.2.3.0/24\\\"\"")
		argNeighGCThresh1                       = pflag.Int("neigh-gc-thresh1", DefaultNeighGCThresh1, "Value to set net.ipv4/ipv6.neigh.default.gc_thresh1")
		argNeighGCThresh2                       = pflag.Int("neigh-gc-thresh2", DefaultNeighGCThresh2, "Value to set net.ipv4/ipv6.neigh.default.gc_thresh2")
		argNeighGCThresh3                       = pflag.Int("neigh-gc-thresh3", DefaultNeighGCThresh3, "Value to set net.ipv4/ipv6.neigh.default.gc_thresh3")
		argExtraNodeLocalVxlanIPCidrs           = pflag.String("extra-node-local-vxlan-ip-cidrs", "", "The cidr list to select node extra local vxlan ip, e.g., \"192.168.10.0/24,10.2.3.0/24\"")
		argEnableVlanArpEnhancement             = pflag.Bool("enable-vlan-arp-enhancement", true, "Whether enable arp source enhancement in a vlan environment")
		argIPv6RouteCacheMaxSize                = pflag.Int("ipv6-route-cache-max-size", DefaultIPv6RouteCacheMaxSize, "Value to set net.ipv6.route.max_size")
		argIPv6RouteCacheGCThresh               = pflag.Int("ipv6-route-cache-gc-thresh", DefaultIPv6RouteCacheGCThresh, "Value to set net.ipv6.route.gc_thresh")
		argPatchCalicoPodIPsAnnotation          = pflag.Bool("patch-calico-pod-ips-annotation", true, "Patch \"cni.projectcalico.org/podIPs\" annotations to pod")
		argCheckPodConnectivityFromHost         = pflag.Bool("check-pod-connectivity-from-host", true, "Check pod's connectivity from host before start it")
	)

	// mute info log for ipset lib
	logrus.SetLevel(logrus.WarnLevel)

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	nodeName := os.Getenv("KUBE_NODE_NAME")
	if nodeName == "" {
		return nil, fmt.Errorf("env KUBE_NODE_NAME not exists")
	}

	config := &Configuration{
		BindSocket:                           *argBindSocket,
		NodeName:                             nodeName,
		NodeVlanIfName:                       *argPreferVlanInterfaces,
		NodeVxlanIfName:                      *argPreferVxlanInterfaces,
		NodeBGPIfName:                        *argPreferBGPInterfaces,
		HealthyServerAddress:                 *argHealthyServerAddress,
		MetricsServerAddress:                 *argMetricsServerAddress,
		BGPgRPCServerAddress:                 *argBGPgRPCServerAddress,
		LocalDirectTableNum:                  *argLocalDirectTableNum,
		ToOverlaySubnetTableNum:              *argToOverlaySubnetTableNum,
		OverlayMarkTableNum:                  *argOverlayMarkTableNum,
		VlanCheckTimeout:                     *argVlanCheckTimeout,
		VxlanUDPPort:                         *argVxlanUDPPort,
		IptablesCheckDuration:                *argIPtablesCheckDuration,
		VxlanBaseReachableTime:               *argVxlanBaseReachableTime,
		NeighGCThresh1:                       *argNeighGCThresh1,
		NeighGCThresh2:                       *argNeighGCThresh2,
		NeighGCThresh3:                       *argNeighGCThresh3,
		VxlanExpiredNeighCachesClearInterval: *argVxlanExpiredNeighCachesClearInterval,
		EnableVlanArpEnhancement:             *argEnableVlanArpEnhancement,
		IPv6RouteCacheMaxSize:                *argIPv6RouteCacheMaxSize,
		IPv6RouteCacheGCThresh:               *argIPv6RouteCacheGCThresh,
		PatchCalicoPodIPsAnnotation:          *argPatchCalicoPodIPsAnnotation,
		CheckPodConnectivityFromHost:         *argCheckPodConnectivityFromHost,
	}

	if *argPreferVlanInterfaces == "" {
		config.NodeVlanIfName = *argPreferInterfaces
	}

	if *argExtraNodeLocalVxlanIPCidrs != "" {
		var err error
		config.ExtraNodeLocalVxlanIPCidrs, err = parseCidrString(*argExtraNodeLocalVxlanIPCidrs)
		if err != nil {
			return nil, fmt.Errorf("failed to parse extra node local vxlan ip cidrs: %v", err)
		}
	}

	if *argVtepAddressCIDRs != "" {
		var err error
		config.VtepAddressCIDRs, err = parseCidrString(*argVtepAddressCIDRs)
		if err != nil {
			return nil, fmt.Errorf("failed to parse vtep address cidrs: %v", err)
		}
	}

	if err := config.initNicConfig(); err != nil {
		return nil, err
	}

	return config, nil
}

func (config *Configuration) initNicConfig() error {
	defaultGatewayIf, err := daemonutils.GetDefaultInterface(netlink.FAMILY_V4)
	if err != nil && err != daemonutils.NotExist {
		return fmt.Errorf("failed to get ipv4 default gateway interface: %v", err)
	} else if err == daemonutils.NotExist {
		// IPv4 default gateway interface not found, check IPv6.
		defaultGatewayIf, err = daemonutils.GetDefaultInterface(netlink.FAMILY_V6)
		if err != nil && err != daemonutils.NotExist {
			return fmt.Errorf("failed to get ipv6 default gateway interface: %v", err)
		}
	}

	if defaultGatewayIf == nil {
		return fmt.Errorf("both ipv4 and ipv6 default gateway not found")
	}

	// if vlan/vxlan interface name is not provided, get the ipv4 default gateway interface
	config.NodeVlanIfName = utils.PickFirstNonEmptyString(config.NodeVlanIfName, defaultGatewayIf.Name)
	config.NodeVxlanIfName = utils.PickFirstNonEmptyString(config.NodeVxlanIfName, defaultGatewayIf.Name)
	config.NodeBGPIfName = utils.PickFirstNonEmptyString(config.NodeBGPIfName, defaultGatewayIf.Name)

	vlanNodeInterface, err := daemonutils.GetInterfaceByPreferString(config.NodeVlanIfName)
	if err != nil {
		return fmt.Errorf("failed to get vlan node interface: %v", err)
	}
	// To update prefer result interface.
	config.NodeVlanIfName = vlanNodeInterface.Name

	vxlanNodeInterface, err := daemonutils.GetInterfaceByPreferString(config.NodeVxlanIfName)
	if err != nil {
		return fmt.Errorf("failed to get vxlan node interface: %v", err)
	}
	// To update prefer result interface.
	config.NodeVxlanIfName = vxlanNodeInterface.Name

	bgpNodeInterface, err := daemonutils.GetInterfaceByPreferString(config.NodeBGPIfName)
	if err != nil {
		return fmt.Errorf("failed to get vxlan node interface: %v", err)
	}
	// To update prefer result interface.
	config.NodeBGPIfName = bgpNodeInterface.Name

	if config.VlanMTU == 0 || config.VlanMTU > vlanNodeInterface.MTU {
		config.VlanMTU = vlanNodeInterface.MTU
	}

	if config.BGPMTU == 0 || config.BGPMTU > bgpNodeInterface.MTU {
		config.BGPMTU = bgpNodeInterface.MTU
	}

	// VXLAN uses a 50-byte header
	if config.VxlanMTU == 0 || config.VxlanMTU > vxlanNodeInterface.MTU-50 {
		config.VxlanMTU = vxlanNodeInterface.MTU - 50
	}

	return nil
}

func parseCidrString(cidrListString string) ([]*net.IPNet, error) {
	var cidrList []*net.IPNet
	cidrStringList := strings.Split(cidrListString, ",")
	for _, cidrString := range cidrStringList {
		_, cidr, err := net.ParseCIDR(cidrString)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cidr %v: %v", cidrString, err)
		}

		cidrList = append(cidrList, cidr)
	}

	return cidrList, nil
}
