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

import (
	"context"
	"fmt"
	"sync"

	ctrl "sigs.k8s.io/controller-runtime"
)

var _ Checker = &checker{}

type checker struct {
	sync.Mutex
	checkMap map[string]Check
}

func (c *checker) Register(name string, check Check) error {
	c.Lock()
	defer c.Unlock()

	if check == nil {
		return fmt.Errorf("check can not be null")
	}

	if _, exist := c.checkMap[name]; exist {
		return fmt.Errorf("check %s has been registered", name)
	}

	c.checkMap[name] = check
	return nil
}

func (c *checker) Unregister(name string) error {
	c.Lock()
	defer c.Unlock()

	delete(c.checkMap, name)
	return nil
}

func (c *checker) CheckAll(ctx context.Context, clusterManager ctrl.Manager, opts ...Option) (map[string]CheckResult, error) {
	c.Lock()
	defer c.Unlock()

	options := ToOptions(opts...)

	if clusterManager == nil {
		return nil, fmt.Errorf("cluster manager can not be null")
	}

	ret := make(map[string]CheckResult)
	for name, check := range c.checkMap {
		// TODO: observe panic to error
		ret[name] = check.Check(ctx, clusterManager, RawOptions(*options))
	}

	return ret, nil
}

func (c *checker) Check(ctx context.Context, name string, clusterManager ctrl.Manager, opts ...Option) (CheckResult, error) {
	c.Lock()
	defer c.Unlock()

	if check, exist := c.checkMap[name]; exist {
		return check.Check(ctx, clusterManager, opts...), nil
	}

	return nil, fmt.Errorf("check %s not found", name)
}

func NewChecker() Checker {
	return &checker{
		Mutex:    sync.Mutex{},
		checkMap: make(map[string]Check),
	}
}
