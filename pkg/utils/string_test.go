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
	"errors"
	"reflect"
	"testing"
)

func TestPickFirstNonEmptyString(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			"no input",
			nil,
			"",
		},
		{
			"some empty string first",
			[]string{"", "", "abc", "def"},
			"abc",
		},
		{
			"normal",
			[]string{"a", "b", "c"},
			"a",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if result := PickFirstNonEmptyString(test.input...); result != test.expected {
				t.Errorf("test %s fail, expected %s but got %s", test.name, test.expected, result)
			}
		})
	}
}

func TestCheckNotEmpty(t *testing.T) {
	tests := []struct {
		name        string
		checkName   string
		checkString string
		err         error
	}{
		{
			name:        "empty",
			checkName:   "haha",
			checkString: "",
			err:         errors.New("haha must not be empty"),
		},
		{
			name:        "non empty",
			checkName:   "haha",
			checkString: "nonempty",
			err:         nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := CheckNotEmpty(test.checkName, test.checkString); !reflect.DeepEqual(err, test.err) {
				t.Errorf("test %s fail, expected %v but got %v", test.name, test.err, err)
			}
		})
	}
}
