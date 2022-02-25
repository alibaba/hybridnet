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

package multicluster

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	"github.com/alibaba/hybridnet/pkg/controllers/utils"
)

var _ manager.Runnable = &RemoteSubnetGarbageCollection{}
var _ manager.Runnable = &RemoteVTEPGarbageCollection{}

type RemoteSubnetGarbageCollection struct {
	eventChan  chan event.GenericEvent
	logger     logr.Logger
	reconciler *RemoteSubnetReconciler
}

func (r *RemoteSubnetGarbageCollection) Start(ctx context.Context) error {
	r.logger.Info("remote subnet garbage collection is starting")

	wait.UntilWithContext(ctx, func(c context.Context) {
		subnetList, err := utils.ListSubnets(r.reconciler.Client)
		if err != nil {
			r.logger.Error(err, "unable to list subnets")
			return
		}
		var expectedRemoteSubnetSet = sets.NewString()
		for i := range subnetList.Items {
			expectedRemoteSubnetSet.Insert(generateRemoteSubnetName(r.reconciler.ClusterName, subnetList.Items[i].Name))
		}

		// TODO: use indexer field instead of label selector
		var currentRemoteSubnetSet = sets.NewString()
		remoteSubnet, err := utils.ListRemoteSubnets(r.reconciler.ParentCluster.GetClient(),
			client.MatchingLabels{constants.LabelCluster: r.reconciler.ClusterName})
		if err != nil {
			r.logger.Error(err, "unable to list remote subnets")
			return
		}
		for i := range remoteSubnet.Items {
			currentRemoteSubnetSet.Insert(remoteSubnet.Items[i].Name)
		}

		redundantRemoteSubnetSet := currentRemoteSubnetSet.Difference(expectedRemoteSubnetSet)
		for _, redundantRemoteSubnet := range redundantRemoteSubnetSet.UnsortedList() {
			// trigger this generic event for the missing (already deleted) objects
			r.eventChan <- event.GenericEvent{
				Object: &networkingv1.Subnet{
					ObjectMeta: metav1.ObjectMeta{
						Name: splitSubnetNameFromRemoteSubnetName(redundantRemoteSubnet),
					},
				},
			}
		}
	}, time.Minute)

	r.logger.Info("remote subnet garbage collection is stopping")
	return nil
}

func (r *RemoteSubnetGarbageCollection) EventChannel() <-chan event.GenericEvent {
	return r.eventChan
}

func NewRemoteSubnetGarbageCollection(logger logr.Logger, reconciler *RemoteSubnetReconciler) *RemoteSubnetGarbageCollection {
	return &RemoteSubnetGarbageCollection{
		eventChan:  make(chan event.GenericEvent, 10),
		logger:     logger,
		reconciler: reconciler,
	}
}

type RemoteVTEPGarbageCollection struct {
	eventChan  chan<- event.GenericEvent
	logger     logr.Logger
	reconciler *RemoteVtepReconciler
}

func (r *RemoteVTEPGarbageCollection) Start(ctx context.Context) error {
	r.logger.Info("remote vtep garbage collection is starting")

	wait.UntilWithContext(ctx, func(c context.Context) {
		nodeNames, err := utils.ListNodesToNames(r.reconciler.Client)
		if err != nil {
			r.logger.Error(err, "unable to list nodes")
			return
		}
		var expectedRemoteVTEPSet = sets.NewString()
		for _, nodeName := range nodeNames {
			expectedRemoteVTEPSet.Insert(generateRemoteSubnetName(r.reconciler.ClusterName, nodeName))
		}

		// TODO: use indexer field instead of label selector
		var currentRemoteVTEPSet = sets.NewString()
		remoteVtepList, err := utils.ListRemoteVteps(r.reconciler.ParentCluster.GetClient(),
			client.MatchingLabels{constants.LabelCluster: r.reconciler.ClusterName})
		if err != nil {
			r.logger.Error(err, "unable to list remote VTEPs")
			return
		}
		for i := range remoteVtepList.Items {
			currentRemoteVTEPSet.Insert(remoteVtepList.Items[i].Name)
		}

		redundantRemoteVTEPSet := currentRemoteVTEPSet.Difference(expectedRemoteVTEPSet)
		for _, redundantRemoteVTEP := range redundantRemoteVTEPSet.UnsortedList() {
			// trigger this generic event for the missing (already deleted) objects
			r.eventChan <- event.GenericEvent{
				Object: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: splitNodeNameFromRemoteVTEPName(redundantRemoteVTEP),
					},
				},
			}
		}
	}, time.Minute)

	r.logger.Info("remote vtep garbage collection is stopping")
	return nil
}

func NewRemoteVTEPGarbageCollection(logger logr.Logger, eventChan chan<- event.GenericEvent, reconciler *RemoteVtepReconciler) *RemoteVTEPGarbageCollection {
	return &RemoteVTEPGarbageCollection{
		eventChan:  eventChan,
		logger:     logger,
		reconciler: reconciler,
	}
}
