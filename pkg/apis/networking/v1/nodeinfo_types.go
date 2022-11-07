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

// NodeInfoSpec defines the desired state of NodeInfo
type NodeInfoSpec struct {
	// vtepInfo is the basic information of this node as a VTEP. Not necessary if no overlay network exist.
	// +kubebuilder:validation:Optional
	VTEPInfo *VTEPInfo `json:"vtepInfo,omitempty"`
}

// NodeInfoStatus defines the observed state of NodeInfo
type NodeInfoStatus struct {
	// +kubebuilder:validation:Optional
	UpdateTimestamp metav1.Time `json:"updateTimestamp,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="VTEPIP",type=string,JSONPath=`.spec.vtepInfo.ip`
// +kubebuilder:printcolumn:name="VTEPMAC",type=string,JSONPath=`.spec.vtepInfo.mac`
// +kubebuilder:printcolumn:name="VTEPLOCALIPS",type=string,JSONPath=`.spec.vtepInfo.localIPs`

// NodeInfo is the Schema for the NodeInfos API
type NodeInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeInfoSpec   `json:"spec,omitempty"`
	Status NodeInfoStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeInfoList contains a list of NodeInfo
type NodeInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeInfo{}, &NodeInfoList{})
}
