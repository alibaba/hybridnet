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

package types

import (
	"os"
	"strings"
	"sync"

	"github.com/alibaba/hybridnet/pkg/feature"
)

type IPFamilyMode string

const (
	IPv4Only  = IPFamilyMode("IPv4Only")
	IPv6Only  = IPFamilyMode("IPv6Only")
	DualStack = IPFamilyMode("DualStack")
)

// short aliases
const (
	IPv4OnlyAlias = "IPv4"
	IPv6OnlyAlias = "IPv6"
)

func ParseIPFamilyFromString(in string) IPFamilyMode {
	// if dual-stack not enabled, only ipv4 subnets will be
	// recognized and promoted
	if !feature.DualStackEnabled() {
		return IPv4Only
	}

	switch strings.ToLower(in) {
	case "":
		return ParseIPFamilyFromEnvOnce()
	case strings.ToLower(string(IPv4Only)), strings.ToLower(IPv4OnlyAlias):
		return IPv4Only
	case strings.ToLower(string(IPv6Only)), strings.ToLower(IPv6OnlyAlias):
		return IPv6Only
	case strings.ToLower(string(DualStack)):
		return DualStack
	default:
		return IPv4Only
	}
}

var ipFamilyFromEnv IPFamilyMode
var ipFamilyFromEnvOnce sync.Once

func ParseIPFamilyFromEnvOnce() IPFamilyMode {
	ipFamilyFromEnvOnce.Do(
		func() {
			ipFamilyFromEnv = ParseIPFamilyFromEnv()
		},
	)
	return ipFamilyFromEnv
}

func ParseIPFamilyFromEnv() IPFamilyMode {
	ipFamilyEnv := os.Getenv("DEFAULT_IP_FAMILY")
	switch strings.ToLower(ipFamilyEnv) {
	case strings.ToLower(string(IPv4Only)), strings.ToLower(IPv4OnlyAlias), "":
		return IPv4Only
	case strings.ToLower(string(IPv6Only)), strings.ToLower(IPv6OnlyAlias):
		return IPv6Only
	case strings.ToLower(string(DualStack)):
		return DualStack
	default:
		return IPFamilyMode(ipFamilyEnv)
	}
}

type NetworkType string

const (
	Underlay = NetworkType("Underlay")
	Overlay  = NetworkType("Overlay")
)

func ParseNetworkTypeFromString(in string) NetworkType {
	switch strings.ToLower(in) {
	case strings.ToLower(string(Underlay)):
		return Underlay
	case strings.ToLower(string(Overlay)):
		return Overlay
	case "":
		return ParseNetworkTypeFromEnvOnce()
	default:
		return NetworkType(in)
	}
}

var networkTypeFromEnv NetworkType
var networkTypeFromEnvOnce sync.Once

func ParseNetworkTypeFromEnvOnce() NetworkType {
	networkTypeFromEnvOnce.Do(
		func() {
			networkTypeFromEnv = ParseNetworkTypeFromEnv()
		},
	)
	return networkTypeFromEnv
}

func ParseNetworkTypeFromEnv() NetworkType {
	networkTypeEnv := os.Getenv("DEFAULT_NETWORK_TYPE")
	switch strings.ToLower(networkTypeEnv) {
	case strings.ToLower(string(Underlay)), "":
		return Underlay
	case strings.ToLower(string(Overlay)):
		return Overlay
	default:
		return NetworkType(networkTypeEnv)
	}
}
