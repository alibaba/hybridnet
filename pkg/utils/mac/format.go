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

import "net"

// NormalizeMAC returns a well formatted hardware address string if mac is valid, or an empty string if not.
func NormalizeMAC(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return ""
	}
	return hw.String()
}
