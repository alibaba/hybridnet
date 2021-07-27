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
// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// RemoteSubnetLister helps list RemoteSubnets.
type RemoteSubnetLister interface {
	// List lists all RemoteSubnets in the indexer.
	List(selector labels.Selector) (ret []*v1.RemoteSubnet, err error)
	// Get retrieves the RemoteSubnet from the index for a given name.
	Get(name string) (*v1.RemoteSubnet, error)
	RemoteSubnetListerExpansion
}

// remoteSubnetLister implements the RemoteSubnetLister interface.
type remoteSubnetLister struct {
	indexer cache.Indexer
}

// NewRemoteSubnetLister returns a new RemoteSubnetLister.
func NewRemoteSubnetLister(indexer cache.Indexer) RemoteSubnetLister {
	return &remoteSubnetLister{indexer: indexer}
}

// List lists all RemoteSubnets in the indexer.
func (s *remoteSubnetLister) List(selector labels.Selector) (ret []*v1.RemoteSubnet, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.RemoteSubnet))
	})
	return ret, err
}

// Get retrieves the RemoteSubnet from the index for a given name.
func (s *remoteSubnetLister) Get(name string) (*v1.RemoteSubnet, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("remotesubnet"), name)
	}
	return obj.(*v1.RemoteSubnet), nil
}
