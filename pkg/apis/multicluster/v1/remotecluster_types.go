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

// RemoteClusterSpec defines the desired state of RemoteCluster
type RemoteClusterSpec struct {
	// APIEndpoint is the API endpoint of the member cluster. This can be a hostname,
	// hostname:port, IP or IP:port.
	// +kubuilder:validation:Required
	APIEndpoint string `json:"apiEndpoint"`
	// CAData holds PEM-encoded bytes (typically read from a root certificates bundle).
	// CAData takes precedence over CAFile
	// +kubuilder:validation:Optional
	CAData []byte `json:"caData,omitempty"`
	// CertData holds PEM-encoded bytes (typically read from a client certificate file).
	// CertData takes precedence over CertFile
	// +kubuilder:validation:Optional
	CertData []byte `json:"certData,omitempty"`
	// KeyData holds PEM-encoded bytes (typically read from a client certificate key file).
	// KeyData takes precedence over KeyFile
	// +kubuilder:validation:Optional
	KeyData []byte `json:"keyData,omitempty"`
	// Timeout is the maximum length of time to wait before giving up on a server request.
	// A value of zero means no timeout.
	Timeout int32 `json:"timeout,omitempty"`
}

// RemoteClusterStatus defines the observed state of RemoteCluster
type RemoteClusterStatus struct {
	// UUID is the unique in time and space value for this object.
	// +kubebuilder:validation:Optional
	UUID types.UID `json:"uuid,omitempty"`
	// State is the current state of cluster.
	// +kubebuilder:validation:Optional
	State ClusterState `json:"state,omitempty"`
	// Conditions represents the observations of a cluster's current state.
	// +kubebuilder:validation:Optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="APIEndpoint",type=string,JSONPath=`.spec.apiEndpoint`
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.status.uuid`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`

// RemoteCluster is the Schema for the remoteclusters API
type RemoteCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteClusterSpec   `json:"spec,omitempty"`
	Status RemoteClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteClusterList contains a list of RemoteCluster
type RemoteClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteCluster{}, &RemoteClusterList{})
}
