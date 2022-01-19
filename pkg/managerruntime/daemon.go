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
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
)

type daemon struct {
	mutex      sync.Mutex
	logger     logr.Logger
	startFunc  func(ctx context.Context) error
	cancelFunc func()
	ctx        context.Context
	daemonStatus
}

type daemonStatus struct {
	running            bool
	restartCount       int32
	terminationMessage string
}

func (s *daemonStatus) Running() bool {
	return s.running
}

func (s *daemonStatus) RestartCount() int32 {
	return s.restartCount
}

func (s *daemonStatus) TerminationMessage() string {
	return s.terminationMessage
}

func (m *managerRuntime) Name() string {
	return m.name
}

func (d *daemon) Run(ctx context.Context) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.running {
		return fmt.Errorf("runtime is running, can not run again")
	}

	d.ctx, d.cancelFunc = context.WithCancel(ctx)
	d.running, d.restartCount, d.terminationMessage = true, 0, ""

	go wait.UntilWithContext(d.ctx, func(ctx context.Context) {
		d.logger.Info("starting daemon")
		if err := d.startFunc(ctx); err != nil {
			d.logger.Error(err, "daemon is exiting")
			d.mutex.Lock()
			d.restartCount++
			d.terminationMessage = err.Error()
			d.mutex.Unlock()
		}
	}, time.Second*30)

	d.logger.Info("daemon is running")
	return nil
}

func (d *daemon) Stop() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if !d.running {
		return nil
	}

	d.cancelFunc()
	d.running, d.restartCount, d.terminationMessage = false, 0, ""

	d.logger.Info("daemon is stopped")
	return nil
}

func (d *daemon) Status() DaemonStatus {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return &daemonStatus{
		running:            d.running,
		restartCount:       d.restartCount,
		terminationMessage: d.terminationMessage,
	}
}
