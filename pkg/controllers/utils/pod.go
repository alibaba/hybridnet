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

import v1 "k8s.io/api/core/v1"

func PodIsEvicted(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodFailed && pod.Status.Reason == "Evicted"
}

func PodIsCompleted(pod *v1.Pod) bool {
	var unknownContainerCount = 0

	hasMatchedStatus := func(name string) bool {
		for i := range pod.Status.ContainerStatuses {
			if pod.Status.ContainerStatuses[i].Name == name {
				return true
			}
		}
		return false
	}

	for i := range pod.Spec.Containers {
		if !hasMatchedStatus(pod.Spec.Containers[i].Name) {
			unknownContainerCount++
		}
	}

	return pod.Status.Phase == v1.PodSucceeded && unknownContainerCount == 0
}
