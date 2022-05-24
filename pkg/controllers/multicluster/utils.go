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

package multicluster

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/alibaba/hybridnet/pkg/controllers/multicluster/clusterchecker"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

func InitClusterStatusChecker(mgr ctrl.Manager) (clusterchecker.Checker, error) {
	clusterUUID, err := utils.GetClusterUUID(mgr.GetClient())
	if err != nil {
		return nil, fmt.Errorf("unable to get cluster UUID: %v", err)
	}

	checker := clusterchecker.NewChecker()

	if err = checker.Register(clusterchecker.HealthzCheckName, &clusterchecker.Healthz{}); err != nil {
		return nil, err
	}
	if err = checker.Register(clusterchecker.BidirectionCheckName, &clusterchecker.Bidirection{LocalUUID: clusterUUID}); err != nil {
		return nil, err
	}
	if err = checker.Register(clusterchecker.OverlayNetIDCheckName, &clusterchecker.OverlayNetID{LocalClient: mgr.GetClient()}); err != nil {
		return nil, err
	}
	if err = checker.Register(clusterchecker.SubnetCheckName, &clusterchecker.Subnet{LocalClient: mgr.GetClient()}); err != nil {
		return nil, err
	}
	return checker, nil
}
