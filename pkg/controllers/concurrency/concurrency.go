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

package concurrency

type Concurrency interface {
	Max() int
	Min() int
}

type ControllerConcurrency int

func (c ControllerConcurrency) Max() int {
	if c > MaxControllerConcurrency {
		return MaxControllerConcurrency
	} else if c > 0 {
		return int(c)
	}
	return 1
}

func (c ControllerConcurrency) Min() int {
	return 1
}
