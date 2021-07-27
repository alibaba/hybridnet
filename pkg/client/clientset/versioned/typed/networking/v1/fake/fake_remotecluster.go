/*
Copyright 2021 The Rama Authors.

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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeRemoteClusters implements RemoteClusterInterface
type FakeRemoteClusters struct {
	Fake *FakeNetworkingV1
}

var remoteclustersResource = schema.GroupVersionResource{Group: "networking.alibaba.com", Version: "v1", Resource: "remoteclusters"}

var remoteclustersKind = schema.GroupVersionKind{Group: "networking.alibaba.com", Version: "v1", Kind: "RemoteCluster"}

// Get takes name of the remoteCluster, and returns the corresponding remoteCluster object, and an error if there is any.
func (c *FakeRemoteClusters) Get(ctx context.Context, name string, options v1.GetOptions) (result *networkingv1.RemoteCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(remoteclustersResource, name), &networkingv1.RemoteCluster{})
	if obj == nil {
		return nil, err
	}
	return obj.(*networkingv1.RemoteCluster), err
}

// List takes label and field selectors, and returns the list of RemoteClusters that match those selectors.
func (c *FakeRemoteClusters) List(ctx context.Context, opts v1.ListOptions) (result *networkingv1.RemoteClusterList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(remoteclustersResource, remoteclustersKind, opts), &networkingv1.RemoteClusterList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &networkingv1.RemoteClusterList{ListMeta: obj.(*networkingv1.RemoteClusterList).ListMeta}
	for _, item := range obj.(*networkingv1.RemoteClusterList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested remoteClusters.
func (c *FakeRemoteClusters) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(remoteclustersResource, opts))
}

// Create takes the representation of a remoteCluster and creates it.  Returns the server's representation of the remoteCluster, and an error, if there is any.
func (c *FakeRemoteClusters) Create(ctx context.Context, remoteCluster *networkingv1.RemoteCluster, opts v1.CreateOptions) (result *networkingv1.RemoteCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(remoteclustersResource, remoteCluster), &networkingv1.RemoteCluster{})
	if obj == nil {
		return nil, err
	}
	return obj.(*networkingv1.RemoteCluster), err
}

// Update takes the representation of a remoteCluster and updates it. Returns the server's representation of the remoteCluster, and an error, if there is any.
func (c *FakeRemoteClusters) Update(ctx context.Context, remoteCluster *networkingv1.RemoteCluster, opts v1.UpdateOptions) (result *networkingv1.RemoteCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(remoteclustersResource, remoteCluster), &networkingv1.RemoteCluster{})
	if obj == nil {
		return nil, err
	}
	return obj.(*networkingv1.RemoteCluster), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeRemoteClusters) UpdateStatus(ctx context.Context, remoteCluster *networkingv1.RemoteCluster, opts v1.UpdateOptions) (*networkingv1.RemoteCluster, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(remoteclustersResource, "status", remoteCluster), &networkingv1.RemoteCluster{})
	if obj == nil {
		return nil, err
	}
	return obj.(*networkingv1.RemoteCluster), err
}

// Delete takes name of the remoteCluster and deletes it. Returns an error if one occurs.
func (c *FakeRemoteClusters) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(remoteclustersResource, name), &networkingv1.RemoteCluster{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeRemoteClusters) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(remoteclustersResource, listOpts)

	_, err := c.Fake.Invokes(action, &networkingv1.RemoteClusterList{})
	return err
}

// Patch applies the patch and returns the patched remoteCluster.
func (c *FakeRemoteClusters) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *networkingv1.RemoteCluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(remoteclustersResource, name, pt, data, subresources...), &networkingv1.RemoteCluster{})
	if obj == nil {
		return nil, err
	}
	return obj.(*networkingv1.RemoteCluster), err
}
