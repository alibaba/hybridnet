/*
 Copyright 2022 The Hybridnet Authors.

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

package mac

import "testing"

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		mac   string
		valid bool
	}{
		{
			"1.2.3.4",
			false,
		},
		{
			"",
			false,
		},
		{
			"11.22.33.44.55",
			false,
		},
		{
			"00:00:00:00",
			false,
		},
		{
			"00-16-EA-AE-3C-40",
			true,
		},
		{
			"08:00:20:0A:8C:6D",
			true,
		},
	}
	for _, test := range tests {
		normalizedMac := NormalizeMAC(test.mac)
		if len(normalizedMac) > 0 && test.valid {
			t.Logf("%s is normalized to %s", test.mac, normalizedMac)
		}
		if (len(normalizedMac) > 0) != test.valid {
			t.Errorf("fail to normalize mac %s", test.mac)
		}
	}
}
