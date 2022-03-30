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
	"errors"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

const BidirectionCheckName = "BidirectionalConnection"

type Bidirection struct {
	LocalUUID types.UID
}

func (b *Bidirection) Check(clusterManager ctrl.Manager, opts ...Option) CheckResult {
	remoteClusterList, err := utils.ListRemoteClusters(clusterManager.GetAPIReader())
	if err != nil {
		return NewResult(err)
	}

	var peered = false
	for i := range remoteClusterList.Items {
		if remoteClusterList.Items[i].Status.UUID == b.LocalUUID {
			peered = true
			break
		}
	}

	if !peered {
		return NewResult(errors.New("remote cluster has no bidrectional connection peered with local cluster"))
	}
	return NewResult(nil)
}
