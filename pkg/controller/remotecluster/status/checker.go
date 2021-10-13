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
	"k8s.io/klog"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

type ConditionChecker func(localObject interface{}, remoteObject interface{}, conditions []v1.ClusterCondition) (
	goOn bool, clusterStatus v1.ClusterStatus)

type CheckerName string

type RegisteredChecker struct {
	Name    CheckerName
	Checker ConditionChecker
}

var ConditionCheckers = []RegisteredChecker{
	{
		Name:    HealthProbe,
		Checker: HealthProbeChecker,
	},
	{
		Name:    LoopbackCheck,
		Checker: LoopbackChecker,
	},
	{
		Name:    OverlayNetIDCheck,
		Checker: OverlayNetIDChecker,
	},
	{
		Name:    BidirectionalConnection,
		Checker: BidirectionalConnectionChecker,
	},
	{
		Name:    SubnetCheck,
		Checker: SubnetChecker,
	},
}

func Check(localObject, remoteObject interface{}, conditions []v1.ClusterCondition) v1.ClusterStatus {
	if len(ConditionCheckers) == 0 {
		return v1.ClusterUnknown
	}

	var clusterStatus v1.ClusterStatus
	var goOn = false

	for i := range ConditionCheckers {
		var registeredChecker = ConditionCheckers[i]

		goOn, clusterStatus = registeredChecker.Checker(localObject, remoteObject, conditions)
		if !goOn {
			klog.Errorf("cluster check break on %s and got cluster status %s", registeredChecker.Name, clusterStatus)
			return clusterStatus
		}
	}

	for i := range conditions {
		if conditions[i].Status == metav1.ConditionTrue {
			klog.Errorf("cluster check failed on %s, will become not ready", conditions[i].Type)
			return v1.ClusterNotReady
		}
	}
	return v1.ClusterReady
}
