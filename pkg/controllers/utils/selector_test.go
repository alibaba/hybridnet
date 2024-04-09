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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSelector(t *testing.T) {
	tests := []struct {
		name          string
		labelSelector *metav1.LabelSelector
		pod           *corev1.Pod
		expectedErr   error
		matched       bool
	}{
		{
			"invalid operator",
			&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "a",
						Operator: "ILLEGAL OPERATOR",
						Values: []string{
							"a",
							"aa",
						},
					},
				},
			},
			nil,
			fmt.Errorf("%q is not a valid pod selector operator", "ILLEGAL OPERATOR"),
			false,
		},
		{
			"nil selector matches no one",
			nil,
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
					},
				},
			},
			nil,
			false,
		},
		{
			"skip nil pod",
			&metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "a",
				},
			},
			nil,
			nil,
			true,
		},
		{
			"match labels",
			&metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "a",
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			true,
		},
		{
			"no match labels",
			&metav1.LabelSelector{
				MatchLabels: map[string]string{
					"c": "c",
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			false,
		},
		{
			"match expressions in",
			&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "a",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"a",
							"aa",
						},
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			true,
		},
		{
			"match expressions not in",
			&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "a",
						Operator: metav1.LabelSelectorOpNotIn,
						Values: []string{
							"m",
						},
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			true,
		},
		{
			"match expressions exist",
			&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "a",
						Operator: metav1.LabelSelectorOpExists,
						Values:   nil,
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			true,
		},
		{
			"match expressions do not exist",
			&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "m",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   nil,
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			true,
		},
		{
			"no match expressions",
			&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "m",
						Operator: metav1.LabelSelectorOpExists,
						Values:   nil,
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
					},
				},
			},
			nil,
			false,
		},
		{
			"match labels and expressions",
			&metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "a",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "b",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"b",
						},
					},
					{
						Key:      "c",
						Operator: metav1.LabelSelectorOpExists,
						Values:   nil,
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
						"c": "c",
					},
				},
			},
			nil,
			true,
		},
		{
			"no match labels and expressions",
			&metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "a",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "b",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"b",
						},
					},
					{
						Key:      "c",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   nil,
					},
				},
			},
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "a",
						"b": "b",
						"c": "c",
					},
				},
			},
			nil,
			false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			podSelector, err := LabelSelectorAsPodSelector(test.labelSelector)
			if !reflect.DeepEqual(err, test.expectedErr) {
				t.Errorf("expected err %v but got %v", test.expectedErr, err)
			}

			if err == nil && podSelector.Matches(test.pod) != test.matched {
				t.Errorf("unexpected matchment %v", podSelector.Matches(test.pod))
			}
		})
	}
}
