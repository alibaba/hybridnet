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

package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

func fillCondition(status *v1.RemoteClusterStatus, toFill *v1.ClusterCondition) {
	if toFill == nil || len(toFill.Type) == 0 {
		return
	}

	idx := -1
	for i := range status.Conditions {
		if status.Conditions[i].Type == toFill.Type {
			idx = i
			break
		}
	}

	now := metav1.Now()
	if idx < 0 {
		toFill.LastProbeTime = now
		toFill.LastTransitionTime = nil
		status.Conditions = append(status.Conditions, *toFill)
		return
	}

	status.Conditions[idx].LastProbeTime = now
	if status.Conditions[idx].Status != toFill.Status {
		status.Conditions[idx].Status = toFill.Status
		// status change brings a transition
		status.Conditions[idx].LastTransitionTime = &now
	}
	status.Conditions[idx].Reason = toFill.Reason
	status.Conditions[idx].Message = toFill.Message
}

func fillStatus(status *v1.RemoteClusterStatus, clusterStatus v1.ClusterStatus) {
	status.Status = clusterStatus
}
