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
// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	internalinterfaces "github.com/alibaba/hybridnet/pkg/client/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// RemoteClusters returns a RemoteClusterInformer.
	RemoteClusters() RemoteClusterInformer
	// RemoteEndpointSlices returns a RemoteEndpointSliceInformer.
	RemoteEndpointSlices() RemoteEndpointSliceInformer
	// RemoteSubnets returns a RemoteSubnetInformer.
	RemoteSubnets() RemoteSubnetInformer
	// RemoteVteps returns a RemoteVtepInformer.
	RemoteVteps() RemoteVtepInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// RemoteClusters returns a RemoteClusterInformer.
func (v *version) RemoteClusters() RemoteClusterInformer {
	return &remoteClusterInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}

// RemoteEndpointSlices returns a RemoteEndpointSliceInformer.
func (v *version) RemoteEndpointSlices() RemoteEndpointSliceInformer {
	return &remoteEndpointSliceInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}

// RemoteSubnets returns a RemoteSubnetInformer.
func (v *version) RemoteSubnets() RemoteSubnetInformer {
	return &remoteSubnetInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}

// RemoteVteps returns a RemoteVtepInformer.
func (v *version) RemoteVteps() RemoteVtepInformer {
	return &remoteVtepInformer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}
