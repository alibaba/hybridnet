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
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

const HealthProbe = CheckerName("BidirectionalConnection")
const ClusterUnhealthy = v1.ClusterConditionType("ClusterUnhealthy")

func HealthProbeChecker(localObject interface{}, remoteObject interface{}, conditions []v1.ClusterCondition) (goOn bool, clusterStatus v1.ClusterStatus) {
	clientInterface, ok := remoteObject.(RemoteHybridnetClient)
	if !ok {
		fillCondition(conditions, healthProbeError("BadRemoteObject", "fail to get hybridnet client from remote object"))
		return false, v1.ClusterOffline
	}
	var hybridnetClient = clientInterface.GetHybridnetClient()
	body, err := hybridnetClient.Discovery().RESTClient().Get().AbsPath("/healthz").Do(context.TODO()).Raw()
	if err != nil {
		fillCondition(conditions, healthProbeError("FailedProbe", err.Error()))
		return false, v1.ClusterNotReady
	}

	if !strings.EqualFold(string(body), "ok") {
		fillCondition(conditions, healthProbeError("ClusterUnhealthy", fmt.Sprintf("unexpected response %s", string(body))))
		return false, v1.ClusterNotReady
	}

	fillCondition(conditions, healthProbeOK("ClusterHealthy", ""))
	return true, ""
}

func healthProbeError(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    ClusterUnhealthy,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func healthProbeOK(reason, message string) *v1.ClusterCondition {
	return &v1.ClusterCondition{
		Type:    ClusterUnhealthy,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}
