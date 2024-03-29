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
	"math"
	"net"
	"strconv"
	"strings"

	"github.com/alibaba/hybridnet/pkg/utils"
)

var (
	ToDNSFormat = utils.ToDNSFormat
)

func ToIPFormat(name string) string {
	const IPv6SeparatorCount = 7
	if isIPv6 := strings.Count(name, "-") == IPv6SeparatorCount; isIPv6 {
		return net.ParseIP(strings.ReplaceAll(name, "-", ":")).String()
	}
	return strings.ReplaceAll(name, "-", ".")
}

func GetIndexFromName(name string) int {
	nameSlice := strings.Split(name, "-")
	indexStr := nameSlice[len(nameSlice)-1]

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return math.MaxInt32
	}
	return index
}
