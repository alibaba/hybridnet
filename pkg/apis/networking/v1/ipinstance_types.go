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
	"k8s.io/apimachinery/pkg/types"
)

// IPInstanceSpec defines the desired state of IPInstance
type IPInstanceSpec struct {
	// +kubebuilder:validation:Required
	Network string `json:"network"`
	// +kubebuilder:validation:Required
	Subnet string `json:"subnet"`
	// +kubebuilder:validation:Required
	Address Address `json:"address"`
	// +kubebuilder:validation:Optional
	Binding Binding `json:"binding,omitempty"`
}

// Binding defines a binding object with necessary info of an IPInstance
type Binding struct {
	// +kubebuilder:validation:Optional
	ReferredObject ObjectMeta `json:"referredObject,omitempty"`

	// +kubebuilder:validation:Optional
	NodeName string `json:"nodeName"`

	// +kubebuilder:validation:Optional
	PodUID types.UID `json:"podUID,omitempty"`

	// +kubebuilder:validation:Optional
	PodName string `json:"podName,omitempty"`

	// +kubebuilder:validation:Optional
	Stateful *StatefulInfo `json:"stateful,omitempty"`
}

// ObjectMeta is a short version of ObjectMeta which is pointing to an Object in specified namespace
type ObjectMeta struct {
	// +kubebuilder:validation:Required
	Kind string `json:"kind,omitempty"`
	// +kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	UID types.UID `json:"uid,omitempty"`
}

// StatefulInfo is a collection of related info if binding to a stateful workload
type StatefulInfo struct {
	Index *int32 `json:"index,omitempty"`
}

// IPInstanceStatus defines the observed state of IPInstance
type IPInstanceStatus struct {
	// +kubebuilder:validation:Optional
	NodeName string `json:"nodeName,omitempty"`
	// DEPRECATED. Planned to remove in v0.6
	// +kubebuilder:validation:Optional
	Phase IPPhase `json:"phase,omitempty"`
	// +kubebuilder:validation:Optional
	PodName string `json:"podName,omitempty"`
	// +kubebuilder:validation:Optional
	PodNamespace string `json:"podNamespace,omitempty"`
	// +kubebuilder:validation:Optional
	SandboxID string `json:"sandboxID,omitempty"`
	// +kubebuilder:validation:Optional
	UpdateTimestamp metav1.Time `json:"updateTimestamp,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.spec.address.ip`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.address.gateway`
// +kubebuilder:printcolumn:name="PodName",type=string,JSONPath=`.spec.binding.podName`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.binding.nodeName`
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`
// +kubebuilder:printcolumn:name="Network",type=string,JSONPath=`.spec.network`

// IPInstance is the Schema for the ipinstances API
type IPInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPInstanceSpec   `json:"spec,omitempty"`
	Status IPInstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IPInstanceList contains a list of IPInstance
type IPInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPInstance{}, &IPInstanceList{})
}
