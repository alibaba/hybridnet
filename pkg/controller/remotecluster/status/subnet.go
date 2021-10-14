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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/utils"
)

const SubnetCheck = CheckerName("SubnetCheck")
const SubnetOverlapped = v1.ClusterConditionType("SubnetOverlapped")

func SubnetChecker(localObject interface{}, remoteObject interface{}, status *v1.RemoteClusterStatus) (goOn bool) {
	localSubnetGetter, ok := localObject.(SubnetGetter)
	if !ok {
		fillCondition(status, subnetError("BadLocalObject", "local object can not support getting subnets"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}
	remoteSubnetGetter, ok := remoteObject.(SubnetGetter)
	if !ok {
		fillCondition(status, subnetError("BadRemoteObject", "remote object can not support getting subnets"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}

	localSubnets, err := localSubnetGetter.ListSubnet()
	if err != nil {
		fillCondition(status, subnetError("FetchFail", fmt.Sprintf("fail to fetch local subnets: %v", err)))
		fillStatus(status, v1.ClusterNotReady)
		return false
	}
	remoteSubnets, err := remoteSubnetGetter.ListSubnet()
	if err != nil {
		fillCondition(status, subnetError("FetchFail", fmt.Sprintf("fail to fetch remote subnets: %v", err)))
		fillStatus(status, v1.ClusterNotReady)
		return false
	}

	for _, localSubnet := range localSubnets {
		for _, remoteSubnet := range remoteSubnets {
			if utils.Intersect(&localSubnet.Spec.Range, &remoteSubnet.Spec.Range) {
				fillCondition(status, subnetError("SubnetOverlapped", fmt.Sprintf("local subnet %s is overlapped with remote subnet %s", localSubnet.Name, remoteSubnet.Name)))
				fillStatus(status, v1.ClusterNotReady)
				return false
			}
		}
	}

	fillCondition(status, subnetOK("NoOverlapped", ""))
	return true
}

func subnetError(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    SubnetOverlapped,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func subnetOK(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    SubnetOverlapped,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
