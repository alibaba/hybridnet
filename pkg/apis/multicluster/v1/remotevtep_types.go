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
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoteVtepSpec defines the desired state of RemoteVtep
type RemoteVtepSpec struct {
	// ClusterName is the name of parent cluster who owns this remote VTEP.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName,omitempty"`
	// NodeName is the name of corresponding node in remote cluster.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName,omitempty"`
	// VTEPInfo is the basic information of this VTEP. Always needed for a remote vtep.
	// +kubebuilder:validation:Required
	networkingv1.VTEPInfo `json:",inline"`
	// EndpointIPList is the IP list of all local endpoints of this VTEP.
	// +kubebuilder:validation:Optional
	EndpointIPList []string `json:"endpointIPList,omitempty"`
}

// RemoteVtepStatus defines the observed state of RemoteVtep
type RemoteVtepStatus struct {
	// LastModifyTime shows the last timestamp when the remote VTEP was updated.
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
// +kubebuilder:printcolumn:name="MAC",type=string,JSONPath=`.spec.mac`
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.spec.ip`
// +kubebuilder:printcolumn:name="NodeName",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="ClusterName",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="LastModifyTime",type=date,JSONPath=`.status.lastModifyTime`

// RemoteVtep is the Schema for the remotevteps API
type RemoteVtep struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteVtepSpec   `json:"spec,omitempty"`
	Status RemoteVtepStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteVtepList contains a list of RemoteVtep
type RemoteVtepList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteVtep `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteVtep{}, &RemoteVtepList{})
}
