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
)

type IPFamilyMode string

const (
	IPv4      = IPFamilyMode("IPv4Only")
	IPv6      = IPFamilyMode("IPv6Only")
	DualStack = IPFamilyMode("DualStack")
)

// short aliases
const (
	IPv4Alias = "IPv4"
	IPv6Alias = "IPv6"
)

func ParseIPFamilyFromString(in string) IPFamilyMode {
	switch strings.ToLower(in) {
	case "":
		return ParseIPFamilyFromEnvOnce()
	case strings.ToLower(string(IPv4)), strings.ToLower(IPv4Alias):
		return IPv4
	case strings.ToLower(string(IPv6)), strings.ToLower(IPv6Alias):
		return IPv6
	case strings.ToLower(string(DualStack)):
		return DualStack
	default:
		return IPFamilyMode(in)
	}
}

func IsValidFamilyMode(ipFamily IPFamilyMode) bool {
	switch ipFamily {
	case IPv4, IPv6, DualStack:
		return true
	default:
		return false
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
	case strings.ToLower(string(IPv4)), strings.ToLower(IPv4Alias), "":
		return IPv4
	case strings.ToLower(string(IPv6)), strings.ToLower(IPv6Alias):
		return IPv6
	case strings.ToLower(string(DualStack)):
		return DualStack
	default:
		return IPFamilyMode(ipFamilyEnv)
	}
}

type NetworkType string

const (
	Underlay  = NetworkType("Underlay")
	Overlay   = NetworkType("Overlay")
	GlobalBGP = NetworkType("GlobalBGP")
)

func ParseNetworkTypeFromString(in string) NetworkType {
	switch strings.ToLower(in) {
	case strings.ToLower(string(Underlay)):
		return Underlay
	case strings.ToLower(string(Overlay)):
		return Overlay
	case strings.ToLower(string(GlobalBGP)):
		return GlobalBGP
	case "":
		return ParseNetworkTypeFromEnvOnce()
	default:
		return NetworkType(in)
	}
}

func IsValidNetworkType(networkType NetworkType) bool {
	switch networkType {
	case Underlay, Overlay, GlobalBGP:
		return true
	default:
		return false
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
	case strings.ToLower(string(GlobalBGP)):
		return GlobalBGP
	default:
		return NetworkType(networkTypeEnv)
	}
}
