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

package mac

import (
	"bytes"
	"testing"
)

func TestGenerateMAC(t *testing.T) {
	mapSet := make(map[string]struct{})

	for i := 0; i < 100; i++ {
		mac := GenerateMAC()
		if !bytes.Equal(mac[:3], hybridnetOUI) {
			t.Errorf("bad mac %s", mac.String())
			return
		}
		if _, exist := mapSet[mac.String()]; exist {
			t.Errorf("dup mac %s", mac.String())
			return
		}
		mapSet[mac.String()] = struct{}{}
	}
}
