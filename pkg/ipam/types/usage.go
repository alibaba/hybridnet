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

package types

type Usage struct {
	Total          uint32
	Used           uint32
	Available      uint32
	LastAllocation string
}

func (u *Usage) Add(in *Usage) {
	if in == nil {
		return
	}

	u.Total += in.Total
	u.Used += in.Used
	u.Available += in.Available
	if len(u.LastAllocation) == 0 {
		u.LastAllocation = in.LastAllocation
	}
}

type NetworkUsage struct {
	LastAllocation string
	Usages         map[IPFamilyMode]*Usage
	SubnetUsages   map[string]*Usage
}

func (n *NetworkUsage) GetByType(ipFamily IPFamilyMode) *Usage {
	if len(n.Usages) > 0 {
		return n.Usages[ipFamily]
	}
	return nil
}

func (n *NetworkUsage) GetBySubnet(subnetName string) *Usage {
	if len(n.SubnetUsages) > 0 {
		return n.SubnetUsages[subnetName]
	}
	return nil
}
