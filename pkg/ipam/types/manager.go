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

import "k8s.io/apimachinery/pkg/types"

type PodInfo struct {
	types.NamespacedName
	IPFamily IPFamilyMode
}

type SubnetIPSuite struct {
	Subnet string
	IP     string
}

func AssignIPOfSubnet(subnet, ip string) SubnetIPSuite {
	return SubnetIPSuite{
		Subnet: subnet,
		IP:     ip,
	}
}

func AssignIP(ip string) SubnetIPSuite {
	return SubnetIPSuite{
		IP: ip,
	}
}

func ReleaseIPOfSubnet(subnet, ip string) SubnetIPSuite {
	return SubnetIPSuite{
		Subnet: subnet,
		IP:     ip,
	}
}
