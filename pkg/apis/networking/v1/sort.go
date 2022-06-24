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

import "sort"

type IPInstancePointerSlice []*IPInstance

func (I IPInstancePointerSlice) Len() int {
	return len(I)
}

func (I IPInstancePointerSlice) Less(i, j int) bool {
	if I[i] == nil || I[j] == nil {
		return false
	}

	// sorting IPInstance by order rule, ipv4 after ipv6
	// so if i is ipv4, it must be less equal than j
	return !IsIPv6IPInstance(I[i])
}

func (I IPInstancePointerSlice) Swap(i, j int) {
	I[i], I[j] = I[j], I[i]
}

func SortIPInstancePointerSlice(in []*IPInstance) {
	sort.Stable(IPInstancePointerSlice(in))
}
