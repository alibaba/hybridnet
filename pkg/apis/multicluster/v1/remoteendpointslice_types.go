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
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoteEndpointSliceSpec defines the desired state of RemoteEndpointSlice, it's a copy of discovery.EndpointSlice
type RemoteEndpointSliceSpec struct {
	// +kubebuilder:validation:Required
	RemoteService RemoteServiceInfo `json:"remoteService"`

	// addressType specifies the type of address carried by this EndpointSlice.
	// All addresses in this slice must be the same type. This field is
	// immutable after creation. The following address types are currently
	// supported:
	// * IPv4: Represents an IPv4 Address.
	// * IPv6: Represents an IPv6 Address.
	// * FQDN: Represents a Fully Qualified Domain Name.
	// +kubebuilder:validation:Required
	AddressType discoveryv1beta1.AddressType `json:"addressType" protobuf:"bytes,4,rep,name=addressType"`
	// endpoints is a list of unique endpoints in this slice. Each slice may
	// include a maximum of 1000 endpoints.
	// +listType=atomic
	// +kubebuilder:validation:Optional
	Endpoints []discoveryv1beta1.Endpoint `json:"endpoints" protobuf:"bytes,2,rep,name=endpoints"`
	// ports specifies the list of network ports exposed by each endpoint in
	// this slice. Each port must have a unique name. When ports is empty, it
	// indicates that there are no defined ports. When a port is defined with a
	// nil port value, it indicates "all ports". Each slice may include a
	// maximum of 100 ports.
	// +optional
	// +listType=atomic
	Ports []discoveryv1beta1.EndpointPort `json:"ports" protobuf:"bytes,3,rep,name=ports"`
}

// RemoteEndpointSliceStatus defines the observed state of RemoteEndpointSlice
type RemoteEndpointSliceStatus struct {
	// LastModifyTime shows the last timestamp when the remote subnet was updated.
	// +kubebuilder:validation:Optional
	LastModifyTime metav1.Time `json:"lastModifyTime,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.spec.remoteService.name`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.remoteService.namespace`
// +kubebuilder:printcolumn:name="AddressType",type=string,JSONPath=`.spec.addressType`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.remoteService.cluster`

// RemoteEndpointSlice is the Schema for the remoteendpointslice API
type RemoteEndpointSlice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteEndpointSliceSpec   `json:"spec,omitempty"`
	Status RemoteEndpointSliceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteEndpointSliceList contains a list of RemoteEndpointSlice
type RemoteEndpointSliceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteEndpointSlice `json:"items"`
}

type RemoteServiceInfo struct {
	// +kubebuilder:validation:Required
	Cluster string `json:"cluster"`

	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

func init() {
	SchemeBuilder.Register(&RemoteEndpointSlice{}, &RemoteEndpointSliceList{})
}
