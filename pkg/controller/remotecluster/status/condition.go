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

func fillCondition(conditions []v1.ClusterCondition, toFill *v1.ClusterCondition) {
	if toFill == nil || len(toFill.Type) == 0 {
		return
	}

	idx := -1
	for i := range conditions {
		if conditions[i].Type == toFill.Type {
			idx = i
			break
		}
	}

	now := metav1.Now()
	if idx < 0 {
		toFill.LastProbeTime = now
		toFill.LastTransitionTime = nil
		conditions = append(conditions, *toFill)
		return
	}

	conditions[idx].LastProbeTime = now
	if conditions[idx].Status != toFill.Status {
		conditions[idx].Status = toFill.Status
		// status change brings a transition
		conditions[idx].LastTransitionTime = &now
	}
	conditions[idx].Reason = toFill.Reason
	conditions[idx].Message = toFill.Message
}
