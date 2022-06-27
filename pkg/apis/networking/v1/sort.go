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
	// if i is ipv4 and j is ipv6, less, return true
	// if i and j are both ipv4 or ipv6, equal, return false
	// if i is ipv6 and j is ipv4, greater, return false
	return !IsIPv6IPInstance(I[i]) && IsIPv6IPInstance(I[j])
}

func (I IPInstancePointerSlice) Swap(i, j int) {
	I[i], I[j] = I[j], I[i]
}

func SortIPInstancePointerSlice(in []*IPInstance) {
	sort.Stable(IPInstancePointerSlice(in))
}
