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

package rcmanager

import "sync"

type SubnetSet interface {
	Remember(subnet string)
	Recognize(subnet string) bool
	Forget(subnet string)
}

type subnetSet struct {
	sync.RWMutex
	data         map[string]struct{}
	callbackFunc func()
}

func NewSubnetSet(callbackFun func()) SubnetSet {
	return &subnetSet{
		RWMutex:      sync.RWMutex{},
		data:         make(map[string]struct{}),
		callbackFunc: callbackFun,
	}
}

func (s *subnetSet) Remember(subnet string) {
	s.Lock()
	defer s.Unlock()

	if _, exist := s.data[subnet]; exist {
		return
	}

	s.data[subnet] = struct{}{}
	s.callbackFunc()
}

func (s *subnetSet) Forget(subnet string) {
	s.Lock()
	defer s.Unlock()

	if _, exist := s.data[subnet]; !exist {
		return
	}

	delete(s.data, subnet)
	s.callbackFunc()
}

func (s *subnetSet) Recognize(subnet string) bool {
	s.RLock()
	defer s.RUnlock()

	_, exist := s.data[subnet]
	return exist
}
