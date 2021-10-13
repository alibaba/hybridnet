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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

const BidirectionalConnection = CheckerName("BidirectionalConnection")
const MissingPeer = v1.ClusterConditionType("MissingPeer")

func BidirectionalConnectionChecker(localObject interface{}, remoteObject interface{}, conditions []v1.ClusterCondition) (goOn bool, clusterStatus v1.ClusterStatus) {
	localUUID, ok := localObject.(LocalUUID)
	if !ok {
		fillCondition(conditions, bidirectionalConnectionError("BadLocalObject", "local object can not support getting uuid"))
		return false, v1.ClusterOffline
	}

	clientInterface, ok := remoteObject.(RemoteHybridnetClient)
	if !ok {
		fillCondition(conditions, bidirectionalConnectionError("BadRemoteObject", "remote object can not support getting hybridnet client"))
		return false, v1.ClusterOffline
	}

	var hybridnetClient = clientInterface.GetHybridnetClient()
	var uuid = localUUID.GetUUID()

	remoteClusterList, err := hybridnetClient.NetworkingV1().RemoteClusters().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fillCondition(conditions, bidirectionalConnectionError("BadConnection", "fail to get remote clusters"))
		return false, v1.ClusterOffline
	}

	var peered = false
	for _, v := range remoteClusterList.Items {
		// has not set uuid, check next time
		if v.Status.UUID == "" {
			continue
		}
		if v.Status.UUID == uuid {
			peered = true
			break
		}
	}

	if !peered {
		fillCondition(conditions, bidirectionalConnectionError("PeerNotFound", "remote cluster has no peered connection pointed here"))
		return false, v1.ClusterNotReady
	}

	fillCondition(conditions, bidirectionalConnectionOK("Established", ""))
	return true, ""
}

func bidirectionalConnectionError(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    MissingPeer,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func bidirectionalConnectionOK(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    MissingPeer,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
