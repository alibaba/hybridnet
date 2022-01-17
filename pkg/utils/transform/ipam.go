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

package transform

import (
	"net"

	v1 "github.com/alibaba/hybridnet/apis/networking/v1"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
	"github.com/alibaba/hybridnet/pkg/utils"
)

func TransferSubnetForIPAM(in *v1.Subnet) *ipamtypes.Subnet {
	_, cidr, _ := net.ParseCIDR(in.Spec.Range.CIDR)

	return ipamtypes.NewSubnet(in.Name,
		in.Spec.Network,
		int32pTouint32p(in.Spec.NetID),
		net.ParseIP(in.Spec.Range.Start),
		net.ParseIP(in.Spec.Range.End),
		net.ParseIP(in.Spec.Range.Gateway),
		cidr,
		utils.StringSliceToMap(in.Spec.Range.ReservedIPs),
		utils.StringSliceToMap(in.Spec.Range.ExcludeIPs),
		net.ParseIP(in.Status.LastAllocatedIP),
		v1.IsPrivateSubnet(in),
		v1.IsIPv6Subnet(in),
	)
}

func TransferNetworkForIPAM(in *v1.Network) *ipamtypes.Network {
	return ipamtypes.NewNetwork(in.Name,
		int32pTouint32p(in.Spec.NetID),
		in.Status.LastAllocatedSubnet,
		ipamtypes.ParseNetworkTypeFromString(string(v1.GetNetworkType(in))),
	)
}

func TransferIPInstanceForIPAM(in *v1.IPInstance) *ipamtypes.IP {
	return &ipamtypes.IP{
		Address:      utils.StringToIPNet(in.Spec.Address.IP),
		Gateway:      net.ParseIP(in.Spec.Address.Gateway),
		NetID:        int32pTouint32p(in.Spec.Address.NetID),
		Subnet:       in.Spec.Subnet,
		Network:      in.Spec.Network,
		PodName:      in.Status.PodName,
		PodNamespace: in.Status.PodNamespace,
		Status:       string(in.Status.Phase),
	}
}

func TransferIPInstancesForIPAM(ips []*v1.IPInstance) []*ipamtypes.IP {
	ret := make([]*ipamtypes.IP, len(ips))
	for idx, ip := range ips {
		ret[idx] = TransferIPInstanceForIPAM(ip)
	}
	return ret
}

func int32pTouint32p(in *int32) *uint32 {
	if in == nil {
		return nil
	}
	temp := uint32(*in)
	return &temp
}
