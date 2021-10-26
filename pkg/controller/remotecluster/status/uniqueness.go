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

const UniquenessCheck = CheckerName("UniquenessCheck")
const ClusterDuplication = v1.ClusterConditionType("ClusterDuplication")

func UniquenessChecker(localObject interface{}, remoteObject interface{}, status *v1.RemoteClusterStatus) (goOn bool) {
	uuidLocker, ok := localObject.(UUIDLocker)
	if !ok {
		fillCondition(status, uniquenessError("BadLocalObject", "local object can not cast to UUID locker"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}
	remoteUUIDGetter, ok := remoteObject.(UUIDGetter)
	if !ok {
		fillCondition(status, uniquenessError("BadRemoteObject", "remote object can not cast to UUID getter"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}
	remoteClusterNameGetter, ok := remoteObject.(ClusterNameGetter)
	if !ok {
		fillCondition(status, uniquenessError("BadRemoteObject", "remote object can not cast to cluster name getter"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}

	remoteUUID, remoteClusterName := remoteUUIDGetter.GetUUID(), remoteClusterNameGetter.GetClusterName()
	if remoteUUID == "" || remoteClusterName == "" {
		fillCondition(status, uniquenessError("InvalidRemoteCluster", "empty UUID or name of remote cluster"))
		fillStatus(status, v1.ClusterNotReady)
		return false
	}

	if err := uuidLocker.Lock(remoteUUID, remoteClusterName); err != nil {
		fillCondition(status, uniquenessError("ClusterDuplication", fmt.Sprintf("fail to lock UUID: %v", err)))
		fillStatus(status, v1.ClusterNotReady)
		return false
	}

	fillCondition(status, uniquenessOK("UniqueCluster", ""))
	return true
}

func uniquenessError(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    ClusterDuplication,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func uniquenessOK(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    ClusterDuplication,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
