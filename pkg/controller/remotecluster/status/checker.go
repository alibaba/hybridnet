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

type ConditionChecker func(localObject interface{}, remoteObject interface{}, status *v1.RemoteClusterStatus) (goOn bool)

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

func Check(localObject, remoteObject interface{}, status *v1.RemoteClusterStatus) {
	if len(ConditionCheckers) == 0 {
		status.Status = v1.ClusterUnknown
		return
	}

	var goOn bool
	for i := range ConditionCheckers {
		var registeredChecker = ConditionCheckers[i]

		goOn = registeredChecker.Checker(localObject, remoteObject, status)
		if !goOn {
			klog.Errorf("cluster check break on %s and got cluster status %s", registeredChecker.Name, status.Status)
			return
		}
	}

	for i := range status.Conditions {
		if status.Conditions[i].Status == metav1.ConditionTrue {
			klog.Errorf("cluster check failed on %s, will become not ready", status.Conditions[i].Type)
			status.Status = v1.ClusterNotReady
			return
		}
	}

	status.Status = v1.ClusterReady
}
