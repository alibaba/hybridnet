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
	"encoding/json"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSortIPInstancePointerSlice(t *testing.T) {
	tests := []struct {
		name   string
		data   []*IPInstance
		expect []*IPInstance
	}{
		{
			"nil",
			nil,
			nil,
		},
		{
			"v4 v6",
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
			},
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
			},
		},
		{
			"v6 v4",
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
			},
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
			},
		},
		{
			"v4 stable",
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4-1",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4-2",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
			},
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4-1",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v4-2",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv4,
						},
					},
				},
			},
		},
		{
			"v6 stable",
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6-1",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6-2",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
			},
			[]*IPInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6-1",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "v6-2",
					},
					Spec: IPInstanceSpec{
						Address: Address{
							Version: IPv6,
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SortIPInstancePointerSlice(test.data)
			if !reflect.DeepEqual(test.data, test.expect) {
				jsonMarshal := func(in interface{}) string {
					bytes, _ := json.Marshal(in)
					return string(bytes)
				}
				t.Errorf("test %s fail, expect %s but got %s", test.name, jsonMarshal(test.expect), jsonMarshal(test.data))
			}
		})
	}
}
