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

package types

import (
	"os"
	"strings"
)

type IPFamilyMode string

const (
	IPv4Only  = IPFamilyMode("IPv4Only")
	IPv6Only  = IPFamilyMode("IPv6Only")
	DualStack = IPFamilyMode("DualStack")
)

func ParseIPFamilyFromString(in string) IPFamilyMode {
	switch strings.ToLower(in) {
	case strings.ToLower(string(IPv4Only)):
		return IPv4Only
	case strings.ToLower(string(IPv6Only)):
		return IPv6Only
	case strings.ToLower(string(DualStack)):
		return DualStack
	default:
		return IPv4Only
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
		return ParseNetworkTypeFromEnv()
	default:
		return NetworkType(in)
	}
}

// TODO: remove this defaulting logic if overlay become the general option for more users
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
