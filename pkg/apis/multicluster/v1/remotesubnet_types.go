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

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

// RemoteSubnetSpec defines the desired state of RemoteSubnet
type RemoteSubnetSpec struct {
	// Range is the IP collection of this remote subnet.
	// +kubebuilder:validation:Required
	Range networkingv1.AddressRange `json:"range"`
	// Type is the network type of this remote subnet.
	// Now there are two known types, Overlay and Underlay.
	// +kubebuilder:validation:Required
	Type networkingv1.NetworkType `json:"networkType,omitempty"`
	// ClusterName is the name of parent cluster who owns this remote subnet.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName,omitempty"`
}

// RemoteSubnetStatus defines the observed state of RemoteSubnet
type RemoteSubnetStatus struct {
	// LastModifyTime shows the last timestamp when the remote subnet was updated.
	// +kubebuilder:validation:Optional
	LastModifyTime metav1.Time `json:"lastModifyTime,omitempty"`
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
// +kubebuilder:printcolumn:name="NetworkType",type=string,JSONPath=`.spec.networkType`
// +kubebuilder:printcolumn:name="ClusterName",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="LastModifyTime",type=date,JSONPath=`.status.lastModifyTime`

// RemoteSubnet is the Schema for the remotesubnets API
type RemoteSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteSubnetSpec   `json:"spec,omitempty"`
	Status RemoteSubnetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteSubnetList contains a list of RemoteSubnet
type RemoteSubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteSubnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteSubnet{}, &RemoteSubnetList{})
}
