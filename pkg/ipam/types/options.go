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

package types

type RefreshOption interface {
	ApplyToRefresh(*RefreshOptions)
}

type RefreshOptions struct {
	ForceAll bool
	Networks []string
}

func (r *RefreshOptions) ApplyOptions(opts []RefreshOption) {
	for _, opt := range opts {
		opt.ApplyToRefresh(r)
	}
}

type RefreshNetworks []string

func (r RefreshNetworks) ApplyToRefresh(options *RefreshOptions) {
	options.Networks = r
}

type RefreshForceAll bool

func (r RefreshForceAll) ApplyToRefresh(options *RefreshOptions) {
	options.ForceAll = bool(r)
}

type AllocateOption interface {
	ApplyToAllocate(*AllocateOptions)
}

type AllocateOptions struct {
	Subnets []string
}

func (a *AllocateOptions) ApplyOptions(opts []AllocateOption) {
	for _, opt := range opts {
		opt.ApplyToAllocate(a)
	}
}

type AllocateSubnets []string

func (a AllocateSubnets) ApplyToAllocate(options *AllocateOptions) {
	options.Subnets = a
}

type AssignOption interface {
	ApplyToAssign(options *AssignOptions)
}

type AssignOptions struct {
	Force bool
}

func (a *AssignOptions) ApplyOptions(opts []AssignOption) {
	for _, opt := range opts {
		opt.ApplyToAssign(a)
	}
}

type AssignForce bool

func (a AssignForce) ApplyToAssign(options *AssignOptions) {
	options.Force = bool(a)
}
