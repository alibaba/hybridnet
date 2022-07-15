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

package networking

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	globalutils "github.com/alibaba/hybridnet/pkg/utils"
)

func InitIndexers(mgr ctrl.Manager) (err error) {
	// init node indexer for networks
	if err = mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.Network{},
		IndexerFieldNode, func(obj client.Object) []string {
			network, ok := obj.(*networkingv1.Network)
			if !ok {
				return nil
			}

			switch networkingv1.GetNetworkType(network) {
			case networkingv1.NetworkTypeUnderlay:
				return globalutils.DeepCopyStringSlice(network.Status.NodeList)
			case networkingv1.NetworkTypeOverlay:
				return []string{OverlayNodeName}
			case networkingv1.NetworkTypeGlobalBGP:
				return []string{GlobalBGPNodeName}
			default:
				return nil
			}
		}); err != nil {
		return err
	}

	// init mac indexer for IPInstances
	if err = mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.IPInstance{},
		IndexerFieldMAC, func(obj client.Object) []string {
			ipInstance, ok := obj.(*networkingv1.IPInstance)
			if !ok {
				return nil
			}
			return []string{ipInstance.Spec.Address.MAC}
		}); err != nil {
		return err
	}

	// init network indexer for Subnets
	return mgr.GetFieldIndexer().IndexField(context.TODO(), &networkingv1.Subnet{},
		IndexerFieldNetwork, func(obj client.Object) []string {
			subnet, ok := obj.(*networkingv1.Subnet)
			if !ok {
				return nil
			}

			networkName := subnet.Spec.Network
			if len(networkName) > 0 {
				return []string{networkName}
			}
			return nil
		})
}
