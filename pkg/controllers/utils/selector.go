/*
 Copyright 2024 The Hybridnet Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type PodSelector interface {
	Matches(pod *corev1.Pod) bool
}

type podSelector struct {
	selector labels.Selector
}

func (p *podSelector) Matches(pod *corev1.Pod) bool {
	// non-blocking during exception cases
	if p == nil || pod == nil {
		return true
	}

	return p.selector.Matches(labels.Set(pod.Labels))
}

func LabelSelectorAsPodSelector(labelSelector *metav1.LabelSelector) (PodSelector, error) {
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return nil, err
	}

	return &podSelector{
		selector: selector,
	}, nil
}
