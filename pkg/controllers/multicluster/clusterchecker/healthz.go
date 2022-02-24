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
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
)

const HealthzCheckName = "HealthProbe"

type Healthz struct{}

func (h *Healthz) Check(clusterManager ctrl.Manager, opts ...Option) CheckResult {
	client, err := discovery.NewDiscoveryClientForConfig(clusterManager.GetConfig())
	if err != nil {
		return NewResult(err)
	}

	body, err := client.RESTClient().Get().AbsPath("/healthz").Do(context.TODO()).Raw()
	if err != nil {
		return NewResult(err)
	}

	if !strings.EqualFold(string(body), "ok") {
		return NewResult(fmt.Errorf("unexpected body %s", string(body)))
	}
	return NewResult(nil)
}
