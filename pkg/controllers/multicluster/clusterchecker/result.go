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

package clusterchecker

import "time"

type result struct {
	err       error
	timeStamp time.Time
}

func (r *result) Succeed() bool {
	return r.err == nil
}

func (r *result) Error() error {
	return r.err
}

func (r *result) TimeStamp() time.Time {
	return r.timeStamp
}

func NewResult(err error) CheckResult {
	return &result{
		err:       err,
		timeStamp: time.Now(),
	}
}
