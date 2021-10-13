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

	clientset "github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	"github.com/alibaba/hybridnet/pkg/daemon/containernetwork"
	daemonutils "github.com/alibaba/hybridnet/pkg/daemon/utils"
	"github.com/alibaba/hybridnet/pkg/utils"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/vishvananda/netlink"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

const (
	UserAgent           = "hybridnet-daemon"
	DefaultBindPort     = 11021
	DefaultVxlanUDPPort = 8472

	DefaultVlanCheckTimeout       = 3 * time.Second
	DefaultIptablesCheckDuration  = 5 * time.Second
	DefaultVxlanBaseReachableTime = 5 * time.Second

	DefaultNeighGCThresh1 = 1024
	DefaultNeighGCThresh2 = 2048
	DefaultNeighGCThresh3 = 4096

	DefaultLocalDirectTableNum     = 39999
	DefaultToOverlaySubnetTableNum = 40000
	DefaultOverlayMarkTableNum     = 40001
)

// Configuration is the daemon conf
type Configuration struct {
	BindSocket     string
	KubeConfigFile string
	NodeName       string

	VlanMTU  int
	VxlanMTU int

	NodeVlanIfName             string
	NodeVxlanIfName            string
	ExtraNodeLocalVxlanIPCidrs []*net.IPNet

	BindPort     int
	VxlanUDPPort int

	VlanCheckTimeout       time.Duration
	IptablesCheckDuration  time.Duration
	VxlanBaseReachableTime time.Duration

	// Use fixed table num to mark "local pod direct rule"
	LocalDirectTableNum int

	// Use fixed table num to mark "to overlay pod subnet rule"
	ToOverlaySubnetTableNum int

	// Use fixed table num to mark "overlay mark table rule"
	OverlayMarkTableNum int

	KubeClient      kubernetes.Interface
	HybridnetClient clientset.Interface

	NeighGCThresh1 int
	NeighGCThresh2 int
	NeighGCThresh3 int
}

// ParseFlags will parse cmd args then init kubeClient and configuration
func ParseFlags() (*Configuration, error) {
	var (
		argPreferInterfaces           = pflag.String("prefer-interfaces", "", "[deprecated]The preferred vlan interfaces used to inter-host pod communication, default: the default route interface")
		argPreferVlanInterfaces       = pflag.String("prefer-vlan-interfaces", "", "The preferred vlan interfaces used to inter-host pod communication, default: the default route interface")
		argPreferVxlanInterfaces      = pflag.String("prefer-vxlan-interfaces", "", "The preferred vxlan interfaces used to inter-host pod communication, default: the default route interface")
		argBindSocket                 = pflag.String("bind-socket", "/var/run/hybridnet.sock", "The socket daemon bind to.")
		argKubeConfigFile             = pflag.String("kubeconfig", "", "Path to kubeconfig file with authorization and master location information. If not set use the inCluster token.")
		argBindPort                   = pflag.Int("healthy-server-port", DefaultBindPort, "The port which daemon server bind")
		argLocalDirectTableNum        = pflag.Int("local-direct-table", DefaultLocalDirectTableNum, "The number of local direct routing table")
		argIptableCheckDuration       = pflag.Duration("iptables-check-duration", DefaultIptablesCheckDuration, "The time period for iptables manager to check iptables rules")
		argToOverlaySubnetTableNum    = pflag.Int("to-overlay-table", DefaultToOverlaySubnetTableNum, "The number of to overlay subnet routing table")
		argOverlayMarkTableNum        = pflag.Int("overlay-mark-table", DefaultOverlayMarkTableNum, "The number of overlay mark routing table")
		argVlanCheckTimeout           = pflag.Duration("vlan-check-timeout", DefaultVlanCheckTimeout, "The timeout of vlan network environment check while pod creating")
		argVxlanUDPPort               = pflag.Int("vxlan-udp-port", DefaultVxlanUDPPort, "The local udp port which vxlan tunnel use")
		argVxlanBaseReachableTime     = pflag.Duration("vxlan-base-reachable-time", DefaultVxlanBaseReachableTime, "The time for neigh caches of vxlan device to get STALE from REACHABLE")
		argNeighGCThresh1             = pflag.Int("neigh-gc-thresh1", DefaultNeighGCThresh1, "Value to set net.ipv4/ipv6.neigh.default.gc_thresh1")
		argNeighGCThresh2             = pflag.Int("neigh-gc-thresh2", DefaultNeighGCThresh2, "Value to set net.ipv4/ipv6.neigh.default.gc_thresh2")
		argNeighGCThresh3             = pflag.Int("neigh-gc-thresh3", DefaultNeighGCThresh3, "Value to set net.ipv4/ipv6.neigh.default.gc_thresh3")
		argExtraNodeLocalVxlanIPCidrs = pflag.String("extra-node-local-vxlan-ip-cidrs", "", "Cidrs to select node extra local vxlan ip, e.g., \"192.168.10.0/24,10.2.3.0/24\"")
	)

	// mute info log for ipset lib
	logrus.SetLevel(logrus.WarnLevel)

	_ = flag.Set("alsologtostderr", "true")
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	// Sync the glog and klog flags.
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			_ = f2.Value.Set(value)
		}
	})

	pflag.CommandLine.AddGoFlagSet(klogFlags)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	nodeName := os.Getenv("KUBE_NODE_NAME")
	if nodeName == "" {
		klog.Errorf("env KUBE_NODE_NAME not exists")
		return nil, fmt.Errorf("env KUBE_NODE_NAME not exists")
	}

	config := &Configuration{
		BindSocket:              *argBindSocket,
		KubeConfigFile:          *argKubeConfigFile,
		NodeName:                nodeName,
		NodeVlanIfName:          *argPreferVlanInterfaces,
		NodeVxlanIfName:         *argPreferVxlanInterfaces,
		BindPort:                *argBindPort,
		LocalDirectTableNum:     *argLocalDirectTableNum,
		ToOverlaySubnetTableNum: *argToOverlaySubnetTableNum,
		OverlayMarkTableNum:     *argOverlayMarkTableNum,
		VlanCheckTimeout:        *argVlanCheckTimeout,
		VxlanUDPPort:            *argVxlanUDPPort,
		IptablesCheckDuration:   *argIptableCheckDuration,
		VxlanBaseReachableTime:  *argVxlanBaseReachableTime,
		NeighGCThresh1:          *argNeighGCThresh1,
		NeighGCThresh2:          *argNeighGCThresh2,
		NeighGCThresh3:          *argNeighGCThresh3,
	}

	if *argPreferVlanInterfaces == "" {
		config.NodeVlanIfName = *argPreferInterfaces
	}

	if *argExtraNodeLocalVxlanIPCidrs != "" {
		var err error
		config.ExtraNodeLocalVxlanIPCidrs, err = parseCidrString(*argExtraNodeLocalVxlanIPCidrs)
		if err != nil {
			klog.Errorf("parse extra node local vxlan ip cidrs failed: %v", err)
			return nil, fmt.Errorf("parse extra node local vxlan ip cidrs failed: %v", err)
		}
	}

	if err := config.initNicConfig(); err != nil {
		return nil, err
	}

	if err := config.initKubeClient(); err != nil {
		return nil, err
	}

	klog.Infof("daemon config: %v", config)
	return config, nil
}

func (config *Configuration) initNicConfig() error {
	defaultGatewayIf, err := containernetwork.GetDefaultInterface(netlink.FAMILY_V4)
	if err != nil && err != daemonutils.NotExist {
		return fmt.Errorf("get ipv4 default gateway interface failed: %v", err)
	} else if err == daemonutils.NotExist {
		// IPv4 default gateway interface not found, check IPv6.
		defaultGatewayIf, err = containernetwork.GetDefaultInterface(netlink.FAMILY_V6)
		if err != nil && err != daemonutils.NotExist {
			return fmt.Errorf("get ipv6 default gateway interface failed: %v", err)
		}
	}

	if defaultGatewayIf == nil {
		return fmt.Errorf("both ipv4 and ipv6 default gateway not found")
	}

	// if vlan/vxlan interface name is not provided, get the ipv4 default gateway interface
	config.NodeVlanIfName = utils.PickFirstNonEmptyString(config.NodeVlanIfName, defaultGatewayIf.Name)
	config.NodeVxlanIfName = utils.PickFirstNonEmptyString(config.NodeVxlanIfName, defaultGatewayIf.Name)

	vlanNodeInterface, err := containernetwork.GetInterfaceByPreferString(config.NodeVlanIfName)
	if err != nil {
		return fmt.Errorf("get vlan node interface failed: %v", err)
	}
	// To update prefer result interface.
	config.NodeVlanIfName = vlanNodeInterface.Name

	vxlanNodeInterface, err := containernetwork.GetInterfaceByPreferString(config.NodeVxlanIfName)
	if err != nil {
		return fmt.Errorf("get vxlan node interface failed: %v", err)
	}
	// To update prefer result interface.
	config.NodeVxlanIfName = vxlanNodeInterface.Name

	klog.Infof("use %v as node vlan interface, and use %v as node vxlan interface",
		config.NodeVlanIfName, config.NodeVxlanIfName)

	if config.VlanMTU == 0 || config.VlanMTU > vlanNodeInterface.MTU {
		config.VlanMTU = vlanNodeInterface.MTU
	}

	// VXLAN uses a 50-byte header
	if config.VxlanMTU == 0 || config.VxlanMTU > vxlanNodeInterface.MTU-50 {
		config.VxlanMTU = vxlanNodeInterface.MTU - 50
	}

	return nil
}

func (config *Configuration) initKubeClient() error {
	var cfg *rest.Config
	var err error
	if cfg, err = clientcmd.BuildConfigFromFlags("", config.KubeConfigFile); err != nil {
		klog.Errorf("build config failed %v", err)
		return err
	}

	// NOTE: be careful to avoid request pressure to api-server
	cfg.QPS = 10
	cfg.Burst = 20

	config.HybridnetClient, err = clientset.NewForConfig(rest.AddUserAgent(cfg, UserAgent))
	if err != nil {
		klog.Errorf("init hybridnet client failed %v", err)
		return err
	}

	cfg.ContentType = "application/vnd.kubernetes.protobuf"
	cfg.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	config.KubeClient, err = kubernetes.NewForConfig(rest.AddUserAgent(cfg, UserAgent))
	if err != nil {
		klog.Errorf("init kubernetes client failed %v", err)
		return err
	}

	return nil
}

func parseCidrString(cidrListString string) ([]*net.IPNet, error) {
	var cidrList []*net.IPNet
	cidrStringList := strings.Split(cidrListString, ",")
	for _, cidrString := range cidrStringList {
		_, cidr, err := net.ParseCIDR(cidrString)
		if err != nil {
			return nil, fmt.Errorf("parse cidr %v failed: %v", cidrString, err)
		}

		cidrList = append(cidrList, cidr)
	}

	return cidrList, nil
}
