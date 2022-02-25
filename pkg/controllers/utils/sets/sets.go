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

package sets

import (
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
)

type CallbackSet interface {
	Insert(item string)
	Delete(item string)
	Has(item string) bool
	WithCallback(callbackFunc func()) CallbackSet
}

func NewCallbackSet() CallbackSet {
	return &callbackSet{
		RWMutex:      sync.RWMutex{},
		sets:         sets.NewString(),
		callbackFunc: nil,
	}
}

type callbackSet struct {
	sync.RWMutex

	sets         sets.String
	callbackFunc func()
}

func (c *callbackSet) Insert(item string) {
	c.Lock()
	defer c.Unlock()

	if c.sets.Has(item) {
		return
	}

	c.sets.Insert(item)
	if c.callbackFunc != nil {
		c.callbackFunc()
	}
}

func (c *callbackSet) Delete(item string) {
	c.Lock()
	defer c.Unlock()

	if !c.sets.Has(item) {
		return
	}

	c.sets.Delete(item)
	if c.callbackFunc != nil {
		c.callbackFunc()
	}
}

func (c *callbackSet) Has(item string) bool {
	c.RLock()
	defer c.RUnlock()

	return c.sets.Has(item)
}

func (c *callbackSet) WithCallback(callbackFunc func()) CallbackSet {
	c.Lock()
	defer c.Unlock()

	c.callbackFunc = callbackFunc
	return c
}
