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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RefreshOption interface {
	ApplyToRefresh(*RefreshOptions)
}

// RefreshOptions is a collection of all configurable configs for Refresh action
type RefreshOptions struct {
	// ForceAll will force all registered networks to be refreshed
	ForceAll bool

	// Networks is the specified network list to be refreshed
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

// AllocateOptions is a collection of all configurable configs for Allocate action
type AllocateOptions struct {
	// Subnets is the specified subnet list where IP should be allocated from
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

// AssignOptions is a collection of all configurable configs for Assign action
type AssignOptions struct {
	// Force means this assignment can force to use reserved IP
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

type CoupleOption interface {
	ApplyToCouple(*CoupleOptions)
}

// CoupleOptions is a collection of all configurable configs for Couple action
type CoupleOptions struct {
	// AdditionalLabels will be patched to IP when coupling
	AdditionalLabels map[string]string

	// OwnerReference will replace the owner reference fetched from Pod explicitly
	OwnerReference *metav1.OwnerReference
}

func (c *CoupleOptions) ApplyOptions(opts []CoupleOption) {
	for _, opt := range opts {
		opt.ApplyToCouple(c)
	}
}

type ReCoupleOption interface {
	ApplyToReCouple(*ReCoupleOptions)
}

// ReCoupleOptions is a collection of all configurable configs for ReCouple action
type ReCoupleOptions struct {
	// AdditionalLabels will be patched to IP when recoupling
	AdditionalLabels map[string]string

	// OwnerReference will replace the owner reference fetched from Pod explicitly
	OwnerReference *metav1.OwnerReference
}

func (r *ReCoupleOptions) ApplyOptions(opts []ReCoupleOption) {
	for _, opt := range opts {
		opt.ApplyToReCouple(r)
	}
}

type ReserveOption interface {
	ApplyToReserve(*ReserveOptions)
}

// ReserveOptions is a coolection of all configurable configs for Reserve action
type ReserveOptions struct {
	// DropPodName means this reservation will drop pod name binding,
	// if pod name will change when IP reservation, set it true
	DropPodName bool
}

func (r *ReserveOptions) ApplyOptions(opts []ReserveOption) {
	for _, opt := range opts {
		opt.ApplyToReserve(r)
	}
}

type AdditionalLabels map[string]string

func (a AdditionalLabels) ApplyToReCouple(options *ReCoupleOptions) {
	options.AdditionalLabels = a
}

func (a AdditionalLabels) ApplyToCouple(options *CoupleOptions) {
	options.AdditionalLabels = a
}

type OwnerReference metav1.OwnerReference

func (o OwnerReference) ApplyToReCouple(options *ReCoupleOptions) {
	oCopy := metav1.OwnerReference(o)
	options.OwnerReference = &oCopy

}

func (o OwnerReference) ApplyToCouple(options *CoupleOptions) {
	oCopy := metav1.OwnerReference(o)
	options.OwnerReference = &oCopy
}

func ResetOwnerReference(orig *metav1.OwnerReference) OwnerReference {
	controller := true
	blockOwnerDeletion := false
	return OwnerReference{
		APIVersion:         orig.APIVersion,
		Kind:               orig.Kind,
		Name:               orig.Name,
		UID:                orig.UID,
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

type DropPodName bool

func (d DropPodName) ApplyToReserve(options *ReserveOptions) {
	options.DropPodName = bool(d)
}
