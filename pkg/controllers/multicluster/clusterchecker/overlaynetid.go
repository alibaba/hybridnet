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
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

const OverlayNetIDCheckName = "OverlayNetIDMatch"

type OverlayNetID struct {
	LocalClient client.Client
}

func (o *OverlayNetID) Check(clusterManager ctrl.Manager) CheckResult {
	localOverlayNetID, err := utils.FindOverlayNetworkNetID(o.LocalClient)
	if err != nil {
		return NewResult(err)
	}

	remoteOverlayNetID, err := utils.FindOverlayNetworkNetID(clusterManager.GetClient())
	if err != nil {
		return NewResult(err)
	}

	if *localOverlayNetID == *remoteOverlayNetID {
		return NewResult(nil)
	}

	return NewResult(fmt.Errorf(
		"overlay net id must match between local cluster %d and remote cluster %d",
		*localOverlayNetID,
		*remoteOverlayNetID,
	))
}
