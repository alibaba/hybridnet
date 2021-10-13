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
)

const LoopbackCheck = CheckerName("LoopbackCheck")
const ClusterLoopback = v1.ClusterConditionType("ClusterLoopback")

func LoopbackChecker(localObject interface{}, remoteObject interface{}, conditions []v1.ClusterCondition) (goOn bool, clusterStatus v1.ClusterStatus) {
	localUUIDInterface, ok := localObject.(LocalUUID)
	if !ok {
		fillCondition(conditions, loopbackError("BadLocalObject", "local object can not support getting UUID"))
		return false, v1.ClusterOffline
	}
	remoteUUIDInterface, ok := remoteObject.(RemoteUUID)
	if !ok {
		fillCondition(conditions, loopbackError("BadRemoteObject", "remote object can not support getting UUID"))
		return false, v1.ClusterOffline
	}

	localUUID, remoteUUID := localUUIDInterface.GetUUID(), remoteUUIDInterface.GetUUID()
	if localUUID == "" || remoteUUID == "" {
		fillCondition(conditions, loopbackError("InvalidUUID", fmt.Sprintf("invalid local UUID %s or remote UUID %s", localUUID, remoteUUID)))
		return false, v1.ClusterNotReady
	}

	if localUUID == remoteUUID {
		fillCondition(conditions, loopbackError("InvalidRemoteCluster", "remote cluster can not loopback to local cluster"))
		return false, v1.ClusterNotReady
	}

	fillCondition(conditions, loopbackOK("UniqueCluster", ""))
	return true, ""
}

func loopbackError(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    ClusterLoopback,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func loopbackOK(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    ClusterLoopback,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
