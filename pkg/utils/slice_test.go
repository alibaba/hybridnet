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
	"reflect"
	"testing"
)

func TestStringSliceToMap(t *testing.T) {
	tests := []struct {
		name string
		s    []string
		m    map[string]struct{}
	}{
		{
			"nil slice",
			nil,
			make(map[string]struct{}),
		},
		{
			"empty slice",
			make([]string, 0),
			make(map[string]struct{}),
		},
		{
			"normal slice",
			[]string{"a", "b"},
			map[string]struct{}{
				"a": {},
				"b": {},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if out := StringSliceToMap(test.s); !reflect.DeepEqual(out, test.m) {
				t.Errorf("test %s fails, expected %+v but got %+v", test.name, test.m, out)
				return
			}
		})
	}
}
