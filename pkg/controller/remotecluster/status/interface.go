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

package status

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
)

type UUIDGetter interface {
	GetUUID() types.UID
}

type UUIDLocker interface {
	Lock(uuid types.UID, clusterName string) error
}

type ClusterNameGetter interface {
	GetClusterName() string
}

type OverlayNetIDGetter interface {
	GetOverlayNetID() *uint32
}

type SubnetGetter interface {
	ListSubnet() ([]*v1.Subnet, error)
}

type ClientGetter interface {
	GetHybridnetClient() versioned.Interface
	GetKubeClient() kubernetes.Interface
}
