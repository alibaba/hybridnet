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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SubnetSpec defines the desired state of Subnet
type SubnetSpec struct {
	// +kubebuilder:validation:Required
	Range AddressRange `json:"range"`
	// +kubebuilder:validation:Optional
	NetID *int32 `json:"netID"`
	// +kubebuilder:validation:Required
	Network string `json:"network"`
	// +kubebuilder:validation:Optional
	Config *SubnetConfig `json:"config"`
}

// SubnetStatus defines the observed state of Subnet
type SubnetStatus struct {
	// +kubebuilder:validation:Optional
	Count `json:",inline"`
	// +kubebuilder:validation:Optional
	LastAllocatedIP string `json:"lastAllocatedIP"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
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

// Subnet is the Schema for the subnets API
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec,omitempty"`
	Status SubnetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SubnetList contains a list of Subnet
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Subnet{}, &SubnetList{})
}
