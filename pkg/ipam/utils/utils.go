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

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
	ipamtypes "github.com/alibaba/hybridnet/pkg/ipam/types"
)

func ToDNSLabelFormatName(ip *ipamtypes.IP) string {
	return utils.ToDNSFormat(ip.Address.IP)
}

func GetIndexFromName(name string) int {
	return utils.GetIndexFromName(name)
}

func NewControllerRef(owner metav1.Object, gvk schema.GroupVersionKind, isController, blockOwnerDeletion bool) *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func ExtractIPVersion(ip *ipamtypes.IP) networkingv1.IPVersion {
	if ip.IsIPv6() {
		return networkingv1.IPv6
	}
	return networkingv1.IPv4
}

func Uint32PtoInt32P(in *uint32) *int32 {
	if in == nil {
		return nil
	}

	out := int32(*in)
	return &out
}

func IntToInt32P(in int) *int32 {
	out := int32(in)
	return &out
}
