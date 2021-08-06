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
	"github.com/pkg/errors"
	"k8s.io/klog"
)

type runFunc func(m *Manager) error

var runFuncMap = map[string]runFunc{
	ipam.ControllerName:          runIPAMController,
	remotecluster.ControllerName: runRemoteClusterController,
}

func runIPAMController(m *Manager) error {
	if ipamController == nil {
		return fmt.Errorf("ipam Controller is not initialized")
	}

	go func() {
		if err := ipamController.Run(m.StopEverything); err != nil {
			klog.Fatalf("unexpected controller %s exit: %v", ipam.ControllerName, err)
		}
		klog.Warningf("controller %s exit successfully", ipam.ControllerName)
	}()

	return nil
}

func runRemoteClusterController(m *Manager) error {
	if rcController == nil {
		return errors.New("remote cluster Controller is not initialized")
	}
	go func() {
		if err := rcController.Run(m.StopEverything); err != nil {
			klog.Fatalf("unexpected controller %s exit: %v", remotecluster.ControllerName, err)
		}
		klog.Warningf("controller %s exit successfully", remotecluster.ControllerName)
	}()

	return nil
}

func runControllers(m *Manager) (err error) {
	for name, runFunc := range runFuncMap {
		if err = runFunc(m); err != nil {
			return fmt.Errorf("fail to run controller %s: %v", name, err)
		}
		klog.Infof("running controller %s", name)
	}

	return nil
}
