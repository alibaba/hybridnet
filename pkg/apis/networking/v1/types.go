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

package v1

import (
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// IPInstance is a specification for a IPInstance resource
type IPInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPInstanceSpec   `json:"spec"`
	Status IPInstanceStatus `json:"status"`
}

// IPInstanceSpec is the spec for a IPInstance resource
type IPInstanceSpec struct {
	Network string  `json:"network"`
	Subnet  string  `json:"subnet"`
	Address Address `json:"address"`
}

type Address struct {
	Version IPVersion `json:"version"`
	IP      string    `json:"ip"`
	Gateway string    `json:"gateway"`
	NetID   *uint32   `json:"netID"`
	MAC     string    `json:"mac"`
}

// IPInstanceStatus is the status for a IPInstance resource
type IPInstanceStatus struct {
	NodeName string  `json:"nodeName"`
	Phase    IPPhase `json:"phase"`

	PodName      string `json:"podName"`
	PodNamespace string `json:"podNamespace"`
	SandboxID    string `json:"sandboxID"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IPInstanceList is a list of IPInstance resources
type IPInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IPInstance `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// Subnet is a specification for a Subnet resource
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec"`
	Status SubnetStatus `json:"status"`
}

// SubnetSpec is the spec for a Subnet resource
type SubnetSpec struct {
	Range   AddressRange  `json:"range"`
	NetID   *uint32       `json:"netID"`
	Network string        `json:"network"`
	Config  *SubnetConfig `json:"config"`
}

// SubnetStatus is the status for a Subnet resource
type SubnetStatus struct {
	Count
	LastAllocatedIP string `json:"lastAllocatedIP"`
}

type AddressRange struct {
	Version     IPVersion `json:"version"`
	Start       string    `json:"start,omitempty"`
	End         string    `json:"end,omitempty"`
	CIDR        string    `json:"cidr"`
	Gateway     string    `json:"gateway"`
	ReservedIPs []string  `json:"reservedIPs,omitempty"`
	ExcludeIPs  []string  `json:"excludeIPs,omitempty"`
}

type SubnetConfig struct {
	GatewayType     string   `json:"gatewayType"`
	GatewayNode     string   `json:"gatewayNode"`
	AutoNatOutgoing *bool    `json:"autoNatOutgoing"`
	Private         *bool    `json:"private"`
	AllowSubnets    []string `json:"allowSubnets"`
}

type Count struct {
	Total     uint32 `json:"total"`
	Used      uint32 `json:"used"`
	Available uint32 `json:"available"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SubnetList is a list of Subnet resources
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Subnet `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// Network is a specification for a Network resource
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec"`
	Status NetworkStatus `json:"status"`
}

// NetworkSpec is the spec for a Network resource
type NetworkSpec struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	NetID    *uint32 `json:"netID"`
	SwitchID string  `json:"switchID"`

	Type NetworkType `json:"type,omitempty"`
}

// NetworkStatus is the status for a Network resource
type NetworkStatus struct {
	LastAllocatedSubnet     string   `json:"lastAllocatedSubnet"`
	LastAllocatedIPv6Subnet string   `json:"lastAllocatedIPv6Subnet,omitempty"`
	SubnetList              []string `json:"subnetList"`
	NodeList                []string `json:"nodeList"`
	Statistics              *Count   `json:"statistics"`
	IPv6Statistics          *Count   `json:"ipv6Statistics,omitempty"`
	DualStackStatistics     *Count   `json:"dualStackStatistics,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkList is a list of Network resources
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Network `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// RemoteCluster is a specification for a RemoteCluster resource
type RemoteCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteClusterSpec   `json:"spec"`
	Status RemoteClusterStatus `json:"status"`
}

// RemoteClusterSpec is the spec for a RemoteCluster resource
type RemoteClusterSpec struct {
	ConnConfig APIServerConnConfig `json:"connConfig"`
}

type APIServerConnConfig struct {
	// apiServer address. Format: https://ip:port
	Endpoint   string `json:"endpoint"`
	CABundle   []byte `json:"caBundle"`
	ClientCert []byte `json:"clientCert"`
	ClientKey  []byte `json:"clientKey"`
	// The maximum length of time to wait before giving up on a server
	// request. A value of zero means no timeout. Default: zero second
	Timeout uint32 `json:"timeout"`
}

// RemoteClusterStatus is the status for a RemoteCluster resource
type RemoteClusterStatus struct {
	// Conditions is an array of current cluster conditions.
	Conditions []ClusterCondition `json:"conditions"`
	// A globally unique identifier. Use for validating.
	// Generated by directly using the corresponding remote cluster's namespace "kube-system" uid
	// types.UID is also universally unique identifiers (also known as UUIDs).
	UUID types.UID `json:"uuid"`
}

// ClusterCondition describes current state of a cluster.
type ClusterCondition struct {
	// Type of cluster condition, Ready or Offline.
	Type ClusterConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status apiv1.ConditionStatus `json:"status"`
	// Last time the condition was checked.
	LastProbeTime metav1.Time `json:"lastProbeTime"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason *string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message *string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RemoteClusterList is a list of RemoteCluster resources
type RemoteClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RemoteCluster `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// RemoteSubnet is a specification for a RemoteSubnet resource
type RemoteSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteSubnetSpec   `json:"spec"`
	Status RemoteSubnetStatus `json:"status"`
}

// RemoteSubnetSpec is the spec for a RemoteSubnet resource
type RemoteSubnetSpec struct {
	Range       AddressRange `json:"range"`
	Type        NetworkType  `json:"type"`
	ClusterName string       `json:"clusterName"`
}

// RemoteSubnetStatus is the status for a RemoteSubnet resource
type RemoteSubnetStatus struct {
	LastModifyTime metav1.Time `json:"lastModifyTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RemoteSubnetList is a list of RemoteSubnetList resources
type RemoteSubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RemoteSubnet `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// RemoteVtep is a specification for a RemoteVtep resource
type RemoteVtep struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteVtepSpec   `json:"spec"`
	Status RemoteVtepStatus `json:"status"`
}

// RemoteVtepSpec is the spec for a RemoteVtep resource
type RemoteVtepSpec struct {
	ClusterName    string   `json:"clusterName"`
	NodeName       string   `json:"nodeName"`
	VtepIP         string   `json:"vtepIP"`
	VtepMAC        string   `json:"vtepMAC"`
	EndpointIPList []string `json:"endpointIPList"`
}

// RemoteVtepStatus is the status for a RemoteVtep resource
type RemoteVtepStatus struct {
	LastModifyTime metav1.Time `json:"lastModifyTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RemoteVtepList is a list of RemoteVtepList resources
type RemoteVtepList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RemoteVtep `json:"items"`
}
