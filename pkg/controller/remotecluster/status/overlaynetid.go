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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

const OverlayNetIDCheck = CheckerName("OverlayNetIDCheck")
const OverlayNetIDMismatch = v1.ClusterConditionType("OverlayNetIDMismatch")

func OverlayNetIDChecker(localObject interface{}, remoteObject interface{}, status *v1.RemoteClusterStatus) (goOn bool) {
	localOverlayNetIDInterface, ok := localObject.(LocalOverlayNetID)
	if !ok {
		fillCondition(status, overlayNetIDError("BadLocalObject", "local object can not support getting overlay net ID"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}

	remoteOverlayNetIDInterface, ok := remoteObject.(RemoteOverlayNetID)
	if !ok {
		fillCondition(status, overlayNetIDError("BadRemoteObject", "remote object can not support getting overlay net ID"))
		fillStatus(status, v1.ClusterOffline)
		return false
	}

	localOverlayNetID := localOverlayNetIDInterface.GetOverlayNetID()
	remoteOverlayNetID := remoteOverlayNetIDInterface.GetOverlayNetID()

	switch {
	case localOverlayNetID == nil:
		fillCondition(status, overlayNetIDError("InvalidLocalOverlayNetID", "fail to fetch a valid one"))
		fillStatus(status, v1.ClusterNotReady)
		return false
	case remoteOverlayNetID == nil:
		fillCondition(status, overlayNetIDError("InvalidRemoteOverlayNetID", "fail to fetch a valid one"))
		fillStatus(status, v1.ClusterNotReady)
		return false
	case *localOverlayNetID != *remoteOverlayNetID:
		fillCondition(status, overlayNetIDError("OverlayNetIDMismatch", "only support same overlay net ID among clusters"))
		fillStatus(status, v1.ClusterNotReady)
		return false
	default:
		fillCondition(status, overlayNetIDOK("OverlayNetIDCheckPass", ""))
		return true
	}
}

func overlayNetIDError(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    OverlayNetIDMismatch,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func overlayNetIDOK(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    OverlayNetIDMismatch,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
