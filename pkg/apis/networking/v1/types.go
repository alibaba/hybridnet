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

package v1

type NetworkType string

const (
	NetworkTypeUnderlay  = NetworkType("Underlay")
	NetworkTypeOverlay   = NetworkType("Overlay")
	NetworkTypeGlobalBGP = NetworkType("GlobalBGP")
)

type NetworkMode string

const (
	NetworkModeBGP       = NetworkMode("BGP")
	NetworkModeVlan      = NetworkMode("VLAN")
	NetworkModeVxlan     = NetworkMode("VXLAN")
	NetworkModeGlobalBGP = NetworkMode("GlobalBGP")
)

type Count struct {
	// +kubebuilder:validation:Optional
	Total int32 `json:"total"`
	// +kubebuilder:validation:Optional
	Used int32 `json:"used"`
	// +kubebuilder:validation:Optional
	Available int32 `json:"available"`
}

type IPVersion string

const (
	IPv4 = IPVersion("4")
	IPv6 = IPVersion("6")
)

type AddressRange struct {
	// +kubebuilder:validation:Required
	Version IPVersion `json:"version"`
	// +kubebuilder:validation:Optional
	Start string `json:"start,omitempty"`
	// +kubebuilder:validation:Optional
	End string `json:"end,omitempty"`
	// +kubebuilder:validation:Required
	CIDR string `json:"cidr"`
	// +kubebuilder:validation:Optional
	Gateway string `json:"gateway"`
	// +kubebuilder:validation:Optional
	ReservedIPs []string `json:"reservedIPs,omitempty"`
	// +kubebuilder:validation:Optional
	ExcludeIPs []string `json:"excludeIPs,omitempty"`
}

type SubnetConfig struct {
	// +kubebuilder:validation:Optional
	GatewayType string `json:"gatewayType"`
	// +kubebuilder:validation:Optional
	GatewayNode string `json:"gatewayNode"`
	// +kubebuilder:validation:Optional
	AutoNatOutgoing *bool `json:"autoNatOutgoing"`
	// +kubebuilder:validation:Optional
	Private *bool `json:"private"`
	// +kubebuilder:validation:Optional
	AllowSubnets []string `json:"allowSubnets"`
}

type NetworkConfig struct {
	// +kubebuilder:validation:Optional
	BGPPeers []BGPPeer `json:"bgpPeers,omitempty"`
}

type Address struct {
	// +kubebuilder:validation:Required
	Version IPVersion `json:"version"`
	// +kubebuilder:validation:Required
	IP string `json:"ip"`
	// +kubebuilder:validation:Optional
	Gateway string `json:"gateway,omitempty"`
	// +kubebuilder:validation:Required
	NetID *int32 `json:"netID"`
	// +kubebuilder:validation:Required
	MAC string `json:"mac"`
}

type BGPPeer struct {
	// +kubebuilder:validation:Required
	ASN int32 `json:"asn"`
	// +kubebuilder:validation:Required
	Address string `json:"address"`
	// +kubebuilder:validation:Optional
	GracefulRestartSeconds int32 `json:"gracefulRestartSeconds,omitempty"`
	// +kubebuilder:validation:Optional
	Password string `json:"password,omitempty"`
}

type IPPhase string

const (
	IPPhaseUsing    = IPPhase("Using")
	IPPhaseReserved = IPPhase("Reserved")
)
