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

import (
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true

// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.spec.address.ip`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.address.gateway`
// +kubebuilder:printcolumn:name="PodName",type=string,JSONPath=`.status.podName`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.network`

// IPInstance is a specification for a IPInstance resource
type IPInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec IPInstanceSpec `json:"spec"`
	// +optional
	Status IPInstanceStatus `json:"status"`
}

// IPInstanceSpec is the spec for a IPInstance resource
type IPInstanceSpec struct {
	// +kubebuilder:validation:Required
	Network string `json:"network"`

	// +kubebuilder:validation:Required
	Subnet string `json:"subnet"`

	// +kubebuilder:validation:Required
	Address Address `json:"address"`
}

type Address struct {
	// +kubebuilder:validation:Required
	Version IPVersion `json:"version"`

	// +kubebuilder:validation:Required
	IP string `json:"ip"`
	// +kubebuilder:validation:Required
	Gateway string `json:"gateway"`
	// +kubebuilder:validation:Required
	NetID *uint32 `json:"netID"`
	// +kubebuilder:validation:Required
	MAC string `json:"mac"`
}

// IPInstanceStatus is the status for a IPInstance resource
type IPInstanceStatus struct {
	// +optional
	NodeName string `json:"nodeName"`
	// +optional
	Phase IPPhase `json:"phase"`

	// +optional
	PodName string `json:"podName"`
	// +optional
	PodNamespace string `json:"podNamespace"`
	// +optional
	SandboxID string `json:"sandboxID"`
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

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.range.version`
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.range.cidr`
// +kubebuilder:printcolumn:name="Start",type=string,JSONPath=`.spec.range.start`
// +kubebuilder:printcolumn:name="End",type=string,JSONPath=`.spec.range.end`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.range.gateway`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.total`
// +kubebuilder:printcolumn:name="Used",type=integer,JSONPath=`.status.used`
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=`.status.available`
// +kubebuilder:printcolumn:name="NetID",type=integer,JSONPath=`.spec.netID`
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.network`

// Subnet is a specification for a Subnet resource
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec SubnetSpec `json:"spec"`
	// +optional
	Status SubnetStatus `json:"status"`
}

// SubnetSpec is the spec for a Subnet resource
type SubnetSpec struct {
	// +kubebuilder:validation:Required
	Range AddressRange `json:"range"`

	// +optional
	NetID *uint32 `json:"netID"`

	// +kubebuilder:validation:Required
	Network string `json:"network"`

	// +optional
	Config *SubnetConfig `json:"config"`
}

// SubnetStatus is the status for a Subnet resource
type SubnetStatus struct {
	// +optional
	Count `json:",inline"`
	// +optional
	LastAllocatedIP string `json:"lastAllocatedIP"`
}

type AddressRange struct {
	// +kubebuilder:validation:Required
	Version IPVersion `json:"version"`

	// +optional
	Start string `json:"start,omitempty"`
	// +optional
	End string `json:"end,omitempty"`

	// +kubebuilder:validation:Required
	CIDR string `json:"cidr"`
	// +kubebuilder:validation:Required
	Gateway string `json:"gateway"`

	// +optional
	ReservedIPs []string `json:"reservedIPs,omitempty"`
	// +optional
	ExcludeIPs []string `json:"excludeIPs,omitempty"`
}

type SubnetConfig struct {
	// +optional
	GatewayType string `json:"gatewayType"`
	// +optional
	GatewayNode string `json:"gatewayNode"`
	// +optional
	AutoNatOutgoing *bool `json:"autoNatOutgoing"`
	// +optional
	Private *bool `json:"private"`
	// +optional
	AllowSubnets []string `json:"allowSubnets"`
}

type Count struct {
	// +optional
	Total uint32 `json:"total"`
	// +optional
	Used uint32 `json:"used"`
	// +optional
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

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NetID",type=integer,JSONPath=`.spec.netID`
// +kubebuilder:printcolumn:name="SwitchID",type=string,JSONPath=`.spec.switchID`

// Network is a specification for a Network resource
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec NetworkSpec `json:"spec"`
	// +optional
	Status NetworkStatus `json:"status"`
}

// NetworkSpec is the spec for a Network resource
type NetworkSpec struct {
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +optional
	NetID *uint32 `json:"netID"`
	// +optional
	SwitchID string `json:"switchID"`

	// +optional
	Type NetworkType `json:"type,omitempty"`
}

// NetworkStatus is the status for a Network resource
type NetworkStatus struct {
	// +optional
	LastAllocatedSubnet string `json:"lastAllocatedSubnet"`
	// +optional
	LastAllocatedIPv6Subnet string `json:"lastAllocatedIPv6Subnet,omitempty"`
	// +optional
	SubnetList []string `json:"subnetList"`
	// +optional
	NodeList []string `json:"nodeList"`
	// +optional
	Statistics *Count `json:"statistics"`
	// +optional
	IPv6Statistics *Count `json:"ipv6Statistics,omitempty"`
	// +optional
	DualStackStatistics *Count `json:"dualStackStatistics,omitempty"`
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

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.connConfig.endpoint`
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.status.uuid`

// RemoteCluster is a specification for a RemoteCluster resource
type RemoteCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec RemoteClusterSpec `json:"spec"`
	// +optional
	Status RemoteClusterStatus `json:"status"`
}

// RemoteClusterSpec is the spec for a RemoteCluster resource
type RemoteClusterSpec struct {
	// +kubebuilder:validation:Required
	ConnConfig APIServerConnConfig `json:"connConfig"`
}

type APIServerConnConfig struct {
	// apiServer address. Format: https://ip:port
	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint"`

	// +kubebuilder:validation:Required
	CABundle []byte `json:"caBundle"`

	// +kubebuilder:validation:Required
	ClientCert []byte `json:"clientCert"`

	// +kubebuilder:validation:Required
	ClientKey []byte `json:"clientKey"`

	// The maximum length of time to wait before giving up on a server
	// request. A value of zero means no timeout. Default: zero second
	// +optional
	Timeout uint32 `json:"timeout"`
}

// RemoteClusterStatus is the status for a RemoteCluster resource
type RemoteClusterStatus struct {
	// Conditions is an array of current cluster conditions.
	// +optional
	Conditions []ClusterCondition `json:"conditions,omitempty"`

	// Status is the integrated status of the cluster.
	// +optional
	Status ClusterStatus `json:"status,omitempty"`

	// A globally unique identifier. Use for validating.
	// Generated by directly using the corresponding remote cluster's namespace "kube-system" uid
	// types.UID is also universally unique identifiers (also known as UUIDs).
	// +optional
	UUID types.UID `json:"uuid"`
}

// ClusterCondition describes current state of a cluster.
type ClusterCondition struct {
	// Type of cluster condition, Ready or Offline.
	// +optional
	Type ClusterConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	// +optional
	Status apiv1.ConditionStatus `json:"status"`

	// Last time the condition was checked.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime"`

	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
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

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.range.version`
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.range.cidr`
// +kubebuilder:printcolumn:name="Start",type=string,JSONPath=`.spec.range.start`
// +kubebuilder:printcolumn:name="End",type=string,JSONPath=`.spec.range.end`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.range.gateway`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.range.type`
// +kubebuilder:printcolumn:name="ClusterName",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="LastModifyTime",type=date,JSONPath=`.status.lastModifyTime`

// RemoteSubnet is a specification for a RemoteSubnet resource
type RemoteSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec RemoteSubnetSpec `json:"spec"`
	// +optional
	Status RemoteSubnetStatus `json:"status"`
}

// RemoteSubnetSpec is the spec for a RemoteSubnet resource
type RemoteSubnetSpec struct {
	// +kubebuilder:validation:Required
	Range AddressRange `json:"range"`

	// +kubebuilder:validation:Required
	Type NetworkType `json:"type"`

	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`
}

// RemoteSubnetStatus is the status for a RemoteSubnet resource
type RemoteSubnetStatus struct {
	// +optional
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

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="VtepMAC",type=string,JSONPath=`.spec.vtepMAC`
// +kubebuilder:printcolumn:name="VtepIP",type=string,JSONPath=`.spec.vtepIP`
// +kubebuilder:printcolumn:name="NodeName",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="ClusterName",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="LastModifyTime",type=date,JSONPath=`.status.lastModifyTime`

// RemoteVtep is a specification for a RemoteVtep resource
type RemoteVtep struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec RemoteVtepSpec `json:"spec"`
	// +optional
	Status RemoteVtepStatus `json:"status"`
}

// RemoteVtepSpec is the spec for a RemoteVtep resource
type RemoteVtepSpec struct {
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// +kubebuilder:validation:Required
	VtepIP string `json:"vtepIP"`

	// +kubebuilder:validation:Required
	VtepMAC string `json:"vtepMAC"`

	// +optional
	EndpointIPList []string `json:"endpointIPList"`
}

// RemoteVtepStatus is the status for a RemoteVtep resource
type RemoteVtepStatus struct {
	// +optional
	LastModifyTime metav1.Time `json:"lastModifyTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RemoteVtepList is a list of RemoteVtepList resources
type RemoteVtepList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RemoteVtep `json:"items"`
}
