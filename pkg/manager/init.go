/*
  Copyright 2021 The Rama Authors.

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

package manager

import (
	"fmt"

	"github.com/oecp/rama/pkg/controller/ipam"
	"github.com/oecp/rama/pkg/controller/remotecluster"
	"k8s.io/klog"
)

type initFunc func(manager *Manager) error

var initFuncMap = map[string]initFunc{
	ipam.ControllerName:          initIPAMController,
	remotecluster.ControllerName: initRemoteClusterController,
}

var ipamController *ipam.Controller

func initIPAMController(m *Manager) error {
	ipamController = ipam.NewController(
		m.KubeClient,
		m.RamaClient,
		m.InformerFactory.Core().V1().Pods(),
		m.InformerFactory.Core().V1().Nodes(),
		m.RamaInformerFactory.Networking().V1().Networks(),
		m.RamaInformerFactory.Networking().V1().Subnets(),
		m.RamaInformerFactory.Networking().V1().IPInstances(),
	)
	return nil
}

var rcController *remotecluster.Controller

func initRemoteClusterController(m *Manager) error {
	rcController = remotecluster.NewController(
		m.KubeClient,
		m.RamaClient,
		m.RamaInformerFactory.Networking().V1().RemoteClusters(),
		m.RamaInformerFactory.Networking().V1().RemoteSubnets(),
		m.RamaInformerFactory.Networking().V1().Subnets(),
		m.RamaInformerFactory.Networking().V1().RemoteVteps(),
		m.RamaInformerFactory.Networking().V1().Networks(),
	)
	return nil
}

func initControllers(m *Manager) (err error) {
	for name, initFunc := range initFuncMap {
		if err = initFunc(m); err != nil {
			return fmt.Errorf("fail to init controller %s: %v", name, err)
		}
		klog.Infof("initialized controller %s", name)
	}

	return nil
}
