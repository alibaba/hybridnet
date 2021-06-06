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

package controller

import (
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/daemon/iptables"
	"github.com/oecp/rama/pkg/daemon/neigh"
	"github.com/oecp/rama/pkg/daemon/route"
)

func (c *Controller) getRouterManager(ipVersion ramav1.IPVersion) *route.Manager {
	if ipVersion == ramav1.IPv6 {
		return c.routeV6Manager
	}
	return c.routeV4Manager
}

func (c *Controller) getNeighManager(ipVersion ramav1.IPVersion) *neigh.Manager {
	if ipVersion == ramav1.IPv6 {
		return c.neighV6Manager
	}
	return c.neighV4Manager
}

func (c *Controller) getIPtablesManager(ipVersion ramav1.IPVersion) *iptables.Manager {
	if ipVersion == ramav1.IPv6 {
		return c.iptablesV6Manager
	}
	return c.iptablesV4Manager
}
