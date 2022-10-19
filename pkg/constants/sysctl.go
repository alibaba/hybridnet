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

package constants

const (
	ProxyArpSysctl       = "/proc/sys/net/ipv4/conf/%s/proxy_arp"
	ProxyDelaySysctl     = "/proc/sys/net/ipv4/neigh/%s/proxy_delay"
	RouteLocalNetSysctl  = "/proc/sys/net/ipv4/conf/%s/route_localnet"
	IPv4ForwardingSysctl = "/proc/sys/net/ipv4/conf/%s/forwarding"

	RpFilterSysctl = "/proc/sys/net/ipv4/conf/%s/rp_filter"

	IPv4NeighGCThresh1 = "/proc/sys/net/ipv4/neigh/default/gc_thresh1"
	IPv4NeighGCThresh2 = "/proc/sys/net/ipv4/neigh/default/gc_thresh2"
	IPv4NeighGCThresh3 = "/proc/sys/net/ipv4/neigh/default/gc_thresh3"

	IPv6NeighGCThresh1 = "/proc/sys/net/ipv6/neigh/default/gc_thresh1"
	IPv6NeighGCThresh2 = "/proc/sys/net/ipv6/neigh/default/gc_thresh2"
	IPv6NeighGCThresh3 = "/proc/sys/net/ipv6/neigh/default/gc_thresh3"

	ProxyNdpSysctl       = "/proc/sys/net/ipv6/conf/%s/proxy_ndp"
	IPv6ForwardingSysctl = "/proc/sys/net/ipv6/conf/%s/forwarding"

	IPv4AppSolicitSysctl = "/proc/sys/net/ipv4/neigh/%s/app_solicit"
	IPv6AppSolicitSysctl = "/proc/sys/net/ipv6/neigh/%s/app_solicit"

	AcceptDADSysctl = "/proc/sys/net/ipv6/conf/%s/accept_dad"
	AcceptRASysctl  = "/proc/sys/net/ipv6/conf/%s/accept_ra"

	IPv4BaseReachableTimeMSSysctl = "/proc/sys/net/ipv4/neigh/%s/base_reachable_time_ms"
	IPv6BaseReachableTimeMSSysctl = "/proc/sys/net/ipv6/neigh/%s/base_reachable_time_ms"

	IPv6DisableModuleParameter = "/sys/module/ipv6/parameters/disable"
	IPv6DisableSysctl          = "/proc/sys/net/ipv6/conf/%s/disable_ipv6"

	IPv6RouteCacheMaxSizeSysctl = "/proc/sys/net/ipv6/route/max_size"
	IPv6RouteCacheGCThresh      = "/proc/sys/net/ipv6/route/gc_thresh"
)
