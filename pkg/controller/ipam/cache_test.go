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

package ipam

import (
	"testing"

	v1 "github.com/oecp/rama/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oecp/rama/pkg/ipam/types"
)

func TestNetworkCacheOperations(t *testing.T) {
	label1 := map[string]string{"networkId": "1"}
	label2 := map[string]string{"networkId": "2"}

	testCache := NewCache()

	network1 := "network1"
	network2 := "network2"
	subnet1 := "subnet1"
	subnet2 := "subnet2"

	testCache.UpdateNetworkCache(&v1.Network{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: network1,
		},
		Spec: v1.NetworkSpec{
			NodeSelector: label1,
		},
		Status: v1.NetworkStatus{
			Statistics: &v1.Count{
				Total:     254,
				Available: 150,
				Used:      104,
			},
			LastAllocatedSubnet: subnet1,
		},
	})

	testCache.UpdateNetworkCache(&v1.Network{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: network2,
		},
		Spec: v1.NetworkSpec{
			NodeSelector: label2,
		},
		Status: v1.NetworkStatus{
			Statistics: &v1.Count{
				Total:     254,
				Available: 150,
				Used:      104,
			},
			LastAllocatedSubnet: subnet2,
		},
	})

	testCache.UpdateNetworkUsage(network2, &types.Usage{
		Total:          254,
		Used:           151,
		Available:      103,
		LastAllocation: "192.168.1.233",
	})

	testCache.UpdateSubnetUsage(subnet2, &types.Usage{
		Total:          254,
		Used:           151,
		Available:      103,
		LastAllocation: "192.168.1.233",
	})

	if !testCache.MatchNetworkByLabels(network1, label1) {
		t.Fatal("failed to match network by label1")
	}

	if !testCache.MatchNetworkByLabels(network2, label2) {
		t.Fatal("failed to match network by label2")
	}

	if testCache.SelectNetworkByLabels(label1) != network1 {
		t.Fatal("failed to selector network by label1")
	}

	if testCache.SelectNetworkByLabels(label2) != network2 {
		t.Fatal("failed to selector network by label2")
	}

	testCache.RemoveNetworkCache(network1)

	if networkList := testCache.GetNetworkList(); len(networkList) != 1 {
		t.Fatal("remove network cache failed")
	}

}
