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

package bgp

import (
	"net"

	"github.com/go-logr/logr"

	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	apb "google.golang.org/protobuf/types/known/anypb"

	api "github.com/osrg/gobgp/v3/api"
)

var (
	v4Family = &api.Family{
		Afi:  api.Family_AFI_IP,
		Safi: api.Family_SAFI_UNICAST,
	}

	v6Family = &api.Family{
		Afi:  api.Family_AFI_IP6,
		Safi: api.Family_SAFI_UNICAST,
	}

	originAttr, _ = apb.New(&api.OriginAttribute{
		Origin: 0,
	})

	noExportCommunityAttr, _ = apb.New(&api.CommunitiesAttribute{
		Communities: []uint32{
			uint32(bgp.COMMUNITY_NO_EXPORT),
		},
	})
)

type peerInfo struct {
	address                string
	asn                    int
	gracefulRestartSeconds uint32
	password               string
}

func generatePeerConfig(p *peerInfo) *api.Peer {
	return &api.Peer{
		Conf: &api.PeerConf{
			NeighborAddress: p.address,
			PeerAsn:         uint32(p.asn),
			AuthPassword:    p.password,
		},
		GracefulRestart: &api.GracefulRestart{
			Enabled:         true,
			RestartTime:     p.gracefulRestartSeconds,
			DeferralTime:    1,
			LocalRestarting: true,
		},
		AfiSafis: []*api.AfiSafi{
			{
				Config: &api.AfiSafiConfig{
					Family:  &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
					Enabled: true,
				},
				MpGracefulRestart: &api.MpGracefulRestart{
					Config: &api.MpGracefulRestartConfig{
						Enabled: true,
					},
				},
			},
			{
				Config: &api.AfiSafiConfig{
					Family:  &api.Family{Afi: api.Family_AFI_IP6, Safi: api.Family_SAFI_UNICAST},
					Enabled: true,
				},
				MpGracefulRestart: &api.MpGracefulRestart{
					Config: &api.MpGracefulRestartConfig{
						Enabled: true,
					},
				},
			},
		},
	}
}

func getIPFamilyFromIP(ip net.IP) *api.Family {
	if ip.To4() == nil {
		return v6Family
	}
	return v4Family
}

func generatePathForIP(ip, nextHop net.IP) *api.Path {
	if len(ip) == 0 {
		return nil
	}

	isIPv6 := ip.To4() == nil
	prefixBytesLen := uint32(net.IPv4len)
	if isIPv6 {
		prefixBytesLen = net.IPv6len
	}

	nlri, _ := apb.New(&api.IPAddressPrefix{
		Prefix:    ip.String(),
		PrefixLen: prefixBytesLen * 8,
	})

	return &api.Path{
		Family: getIPFamilyFromIP(ip),
		Nlri:   nlri,
		Pattrs: []*apb.Any{generateNextHopAttr(isIPv6, nextHop.String(), nlri), originAttr, noExportCommunityAttr},
	}
}

func generatePathForSubnet(subnet *net.IPNet, nextHop net.IP) *api.Path {
	if subnet == nil {
		return nil
	}

	prefixLen, _ := subnet.Mask.Size()

	nlri, _ := apb.New(&api.IPAddressPrefix{
		Prefix:    subnet.IP.String(),
		PrefixLen: uint32(prefixLen),
	})

	return &api.Path{
		Family: getIPFamilyFromIP(subnet.IP),
		Nlri:   nlri,
		Pattrs: []*apb.Any{generateNextHopAttr(subnet.IP.To4() == nil, nextHop.String(), nlri), originAttr},
	}
}

func generateNextHopAttr(isIPv6 bool, nextHop string, nlri *apb.Any) *apb.Any {
	if isIPv6 {
		v6NextHopAttr, _ := apb.New(&api.MpReachNLRIAttribute{
			Family:   v6Family,
			NextHops: []string{nextHop},
			Nlris:    []*apb.Any{nlri},
		})
		return v6NextHopAttr
	}

	v4NextHopAttr, _ := apb.New(&api.NextHopAttribute{
		NextHop: nextHop,
	})
	return v4NextHopAttr
}

func generatePathListFunc(existSubnetPathMap map[string]*net.IPNet, existIPPathMap map[string]net.IP,
	logger logr.Logger) func(p *api.Destination) {
	return func(p *api.Destination) {
		// only collect the path generated from local
		if p.Paths[0].NeighborIp == "<nil>" {
			ipAddr, cidr, err := net.ParseCIDR(p.Prefix)
			if err != nil {
				logger.Error(err, "failed to parse path prefix", "path-prefix", p.Prefix)
				return
			}

			ones, bits := cidr.Mask.Size()
			// What if the subnet is a /32 or /128 cidr? But maybe it will never happen.
			if ones == bits {
				// this path is generated from ip
				if existIPPathMap != nil {
					existIPPathMap[ipAddr.String()] = ipAddr
				}
			} else {
				// this path is generated from subnet
				if existSubnetPathMap != nil {
					existSubnetPathMap[p.Prefix] = &net.IPNet{
						IP:   ipAddr,
						Mask: cidr.Mask,
					}
				}
			}
		}
	}
}
