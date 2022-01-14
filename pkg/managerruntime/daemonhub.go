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
)

type daemonHub struct {
	ctx context.Context
	sync.RWMutex
	hub map[DaemonID]Daemon
}

func (d *daemonHub) Register(id DaemonID, daemon Daemon) error {
	d.Lock()
	defer d.Unlock()

	if daemon == nil {
		return fmt.Errorf("invalid nil daemon")
	}

	if _, exist := d.hub[id]; exist {
		return fmt.Errorf("daemon %s has been registered", id)
	}
	d.hub[id] = daemon
	return nil
}

func (d *daemonHub) IsRegistered(id DaemonID) bool {
	d.RLock()
	defer d.RUnlock()

	_, exist := d.hub[id]
	return exist
}

func (d *daemonHub) Unregister(id DaemonID) error {
	d.Lock()
	defer d.Unlock()

	daemon, exist := d.hub[id]
	if !exist {
		return nil
	}

	if daemon.Status().Running() {
		return fmt.Errorf("daemon is running, can not be unregistered")
	}

	delete(d.hub, id)
	return nil
}

func (d *daemonHub) Run(id DaemonID) error {
	d.Lock()
	defer d.Unlock()

	daemon, exist := d.hub[id]
	if !exist {
		return fmt.Errorf("daemon %s not found", id)
	}

	// TODO: support custom context for different daemons
	return daemon.Run(d.ctx)
}

func (d *daemonHub) Stop(id DaemonID) error {
	d.Lock()
	defer d.Unlock()

	daemon, exist := d.hub[id]
	if !exist {
		return fmt.Errorf("daemon %s not found", id)
	}

	return daemon.Stop()
}

func NewDaemonHub(ctx context.Context) DaemonHub {
	return &daemonHub{
		ctx:     ctx,
		RWMutex: sync.RWMutex{},
		hub:     make(map[DaemonID]Daemon),
	}
}
