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
	"fmt"
	"net"

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

func (c *Controller) getIPInstanceByAddress(address net.IP) (*ramav1.IPInstance, error) {
	ipInstanceList, err := c.ipInstanceIndexer.ByIndex(ByInstanceIPIndexer, address.String())
	if err != nil {
		return nil, fmt.Errorf("get ip instance by ip %v indexer failed: %v", address.String(), err)
	}

	if len(ipInstanceList) > 1 {
		return nil, fmt.Errorf("get more than one ip instance for ip %v", address.String())
	}

	if len(ipInstanceList) == 1 {
		instance, ok := ipInstanceList[0].(*ramav1.IPInstance)
		if !ok {
			return nil, fmt.Errorf("transform obj to ipinstance failed")
		}

		return instance, nil
	}

	if len(ipInstanceList) == 0 {
		// not found
		return nil, nil
	}

	return nil, fmt.Errorf("ip instance for address %v not found", address.String())
}

func initErrorMessageWrapper(prefix string) func(string, ...interface{}) string {
	return func(format string, args ...interface{}) string {
		return prefix + fmt.Sprintf(format, args...)
	}
}
