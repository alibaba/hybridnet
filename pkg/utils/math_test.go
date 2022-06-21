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

import "testing"

func TestMinUint32(t *testing.T) {
	tests := []struct {
		name   string
		a      uint32
		b      uint32
		theMin uint32
	}{
		{
			name:   "a > b",
			a:      10,
			b:      2,
			theMin: 2,
		},
		{
			name:   "a < b",
			a:      10,
			b:      20020,
			theMin: 10,
		},
		{
			name:   "a == b",
			a:      10,
			b:      10,
			theMin: 10,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if result := MinUint32(test.a, test.b); result != test.theMin {
				t.Errorf("test %s expected %d but got %d", test.name, test.theMin, result)
			}
		})
	}
}
