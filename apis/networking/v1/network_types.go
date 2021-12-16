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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkSpec defines the desired state of Network
type NetworkSpec struct {
	// +kubebuilder:validation:Optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +kubebuilder:validation:Optional
	NetID *int32 `json:"netID"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	SwitchID string `json:"switchID"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	Type NetworkType `json:"type,omitempty"`
}

// NetworkStatus defines the observed state of Network
type NetworkStatus struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	LastAllocatedSubnet string `json:"lastAllocatedSubnet"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	LastAllocatedIPv6Subnet string `json:"lastAllocatedIPv6Subnet,omitempty"`
	// +kubebuilder:validation:Optional
	SubnetList []string `json:"subnetList"`
	// +kubebuilder:validation:Optional
	NodeList []string `json:"nodeList"`
	// +kubebuilder:validation:Optional
	Statistics *Count `json:"statistics"`
	// +kubebuilder:validation:Optional
	IPv6Statistics *Count `json:"ipv6Statistics,omitempty"`
	// +kubebuilder:validation:Optional
	DualStackStatistics *Count `json:"dualStackStatistics,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="NetID",type=integer,JSONPath=`.spec.netID`
// +kubebuilder:printcolumn:name="SwitchID",type=string,JSONPath=`.spec.switchID`

// Network is the Schema for the networks API
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec,omitempty"`
	Status NetworkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkList contains a list of Network
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Network `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Network{}, &NetworkList{})
}
