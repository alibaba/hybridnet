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
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ ManagerRuntime = &managerRuntime{}

type managerRuntime struct {
	sync.RWMutex

	name       string
	logger     logr.Logger
	restConfig *rest.Config
	options    *manager.Options
	initFunc   func(mgr manager.Manager) error

	mgr manager.Manager

	ctx        context.Context
	cancelFunc func()

	daemonStatus
}

func (m *managerRuntime) Manager() manager.Manager {
	m.RLock()
	defer m.RUnlock()

	return m.mgr
}

func (m *managerRuntime) Name() string {
	return m.name
}

func (m *managerRuntime) Run(ctx context.Context) (err error) {
	m.Lock()

	if m.running {
		return fmt.Errorf("runtime is running, can not run again")
	}

	m.ctx, m.cancelFunc = context.WithCancel(ctx)
	m.running, m.restartCount, m.terminationMessage = true, 0, ""

	m.Unlock()

	go wait.UntilWithContext(m.ctx, func(ctx context.Context) {
		m.Lock()

		var err error
		if m.mgr, err = manager.New(m.restConfig, *m.options); err != nil {
			m.logger.Error(err, "unable to create manager")
			m.restartCount++
			m.terminationMessage = err.Error()
			m.Unlock()
			return
		}

		if err = m.initFunc(m.mgr); err != nil {
			m.logger.Error(err, "unable to init manager")
			m.restartCount++
			m.terminationMessage = err.Error()
			m.Unlock()
			return
		}

		m.Unlock()

		m.logger.Info("starting daemon")
		if err = m.mgr.Start(ctx); err != nil {
			m.logger.Error(err, "daemon is exiting")
			m.Lock()
			m.restartCount++
			m.terminationMessage = err.Error()
			m.Unlock()

		}
	}, time.Second*30)

	m.logger.Info("running daemon")
	return nil
}

func (m *managerRuntime) Stop() error {
	m.Lock()
	defer m.Unlock()

	if !m.running {
		return nil
	}

	m.cancelFunc()
	m.running, m.restartCount, m.terminationMessage = false, 0, ""

	m.logger.Info("stopping daemon")
	return nil
}

func (m *managerRuntime) Status() DaemonStatus {
	m.RLock()
	defer m.RUnlock()

	return &daemonStatus{
		running:            m.running,
		restartCount:       m.restartCount,
		terminationMessage: m.terminationMessage,
	}
}

func NewManagerRuntime(name string,
	logger logr.Logger,
	config *rest.Config,
	options *manager.Options,
	initFunc func(mgr manager.Manager) error) (ManagerRuntime, error) {

	mgr, err := manager.New(config, *options)
	if err != nil {
		return nil, fmt.Errorf("unable to create manger: %v", err)
	}

	if err = initFunc(mgr); err != nil {
		return nil, fmt.Errorf("unable to init manager: %v", err)
	}

	return &managerRuntime{
		RWMutex:      sync.RWMutex{},
		name:         name,
		logger:       logger.WithValues("name", name),
		restConfig:   config,
		options:      options,
		initFunc:     initFunc,
		mgr:          mgr,
		daemonStatus: daemonStatus{},
	}, nil
}
