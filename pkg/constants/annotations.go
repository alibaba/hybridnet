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
	AnnotationIPPool   = "networking.alibaba.com/ip-pool"
	AnnotationIPFamily = "networking.alibaba.com/ip-family"

	AnnotationMACPool = "networking.alibaba.com/mac-pool"

	AnnotationIPRetain = "networking.alibaba.com/ip-retain"

	AnnotationGlobalService = "networking.alibaba.com/global-service"

	AnnotationSpecifiedNetwork = "networking.alibaba.com/specified-network"
	AnnotationSpecifiedSubnet  = "networking.alibaba.com/specified-subnet"

	AnnotationNetworkType = "networking.alibaba.com/network-type"

	AnnotationHandledByWebhook = "networking.alibaba.com/handled-by-webhook"

	AnnotationCalicoPodIPs = "cni.projectcalico.org/podIPs"
)
