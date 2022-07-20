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
	LabelCluster = "networking.alibaba.com/cluster"
	LabelSubnet  = "networking.alibaba.com/subnet"
	LabelVM      = "networking.alibaba.com/vm"
	LabelNetwork = "networking.alibaba.com/network"
	LabelNode    = "networking.alibaba.com/node"
	LabelPod     = "networking.alibaba.com/pod"
	LabelPodUID  = "networking.alibaba.com/pod-uid"
	LabelVersion = "networking.alibaba.com/version"

	LabelSpecifiedNetwork = "networking.alibaba.com/specified-network"
	LabelSpecifiedSubnet  = "networking.alibaba.com/specified-subnet"

	LabelAddressQuota          = "networking.alibaba.com/address-quota"
	LabelIPv4AddressQuota      = "networking.alibaba.com/ipv4-address-quota"
	LabelIPv6AddressQuota      = "networking.alibaba.com/ipv6-address-quota"
	LabelDualStackAddressQuota = "networking.alibaba.com/dualstack-address-quota"

	LabelNetworkType = "networking.alibaba.com/network-type"

	LabelUnderlayNetworkAttachment = "networking.alibaba.com/underlay-network-attachment"
	LabelOverlayNetworkAttachment  = "networking.alibaba.com/overlay-network-attachment"
	LabelBGPNetworkAttachment      = "networking.alibaba.com/bgp-network-attachment"

	LabelRemoteCluster = "networking.alibaba.com/remote-cluster"
)

const (
	QuotaNonEmpty = "nonempty"
	QuotaEmpty    = "empty"
)

const (
	Attached = "true"
)
