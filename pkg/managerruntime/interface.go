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

package managerruntime

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type ManagerRuntime interface {
	manager.Manager
	Daemon
	Name() string
}

type Daemon interface {
	Run(ctx context.Context) error
	Stop() error
	Status() DaemonStatus
}

type DaemonStatus interface {
	Running() bool
	RestartCount() int32
	TerminationMessage() string
}

type DaemonID types.UID

type DaemonHub interface {
	Register(id DaemonID, daemon Daemon) error
	IsRegistered(id DaemonID) bool
	Unregister(id DaemonID) error
	Run(id DaemonID) error
	Stop(id DaemonID) error
}
