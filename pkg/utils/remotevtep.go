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

package utils

import (
	"fmt"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

const remoteVtepNameFormat = "%v.%v"

func GenRemoteVtepName(clusterName string, nodeName string) string {
	if nodeName == "" || clusterName == "" {
		return ""
	}
	return fmt.Sprintf(remoteVtepNameFormat, clusterName, nodeName)
}

func NewRemoteVtep(clusterName string, uid types.UID, vtepIP string, macAddr string, nodeLocalVxlanIPList string, nodeName string, endpointIPList []string) *networkingv1.RemoteVtep {
	vtep := &networkingv1.RemoteVtep{
		ObjectMeta: metav1.ObjectMeta{
			Name: GenRemoteVtepName(clusterName, nodeName),
			Labels: map[string]string{
				constants.LabelCluster: clusterName,
			},
			Annotations: map[string]string{
				constants.AnnotationNodeLocalVxlanIPList: nodeLocalVxlanIPList,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: networkingv1.SchemeGroupVersion.String(),
					Kind:       "RemoteCluster",
					Name:       clusterName,
					UID:        uid,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: networkingv1.RemoteVtepSpec{
			ClusterName:    clusterName,
			NodeName:       nodeName,
			VtepIP:         vtepIP,
			VtepMAC:        macAddr,
			EndpointIPList: endpointIPList,
		},
		Status: networkingv1.RemoteVtepStatus{
			LastModifyTime: metav1.Now(),
		},
	}
	return vtep
}
