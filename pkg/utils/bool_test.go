/*
  Copyright 2021 The Rama Authors.

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

func TestParseBoolOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		defaultValue bool
		expected     bool
	}{
		{
			"no input and default true",
			"",
			true,
			true,
		},
		{
			"no input and default false",
			"",
			false,
			false,
		},
		{
			"input true and default true",
			"true",
			true,
			true,
		},
		{
			"input true and default false",
			"true",
			false,
			true,
		},
		{
			"input false and default true",
			"false",
			true,
			false,
		},
		{
			"input false and default false",
			"false",
			false,
			false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.expected != ParseBoolOrDefault(test.in, test.defaultValue) {
				t.Fatalf("test %s fails", test.name)
			}
		})
	}
}
