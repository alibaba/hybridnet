/*
  Copyright 2021 The Rama Authors.

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

package v1

type IPVersion string

const (
	IPv4 = IPVersion("4")
	IPv6 = IPVersion("6")
)

type IPPhase string

const (
	IPPhaseUsing    = IPPhase("Using")
	IPPhaseReserved = IPPhase("Reserved")
)

type NetworkType string

const (
	NetworkTypeUnderlay = NetworkType("Underlay")
	NetworkTypeOverlay  = NetworkType("Overlay")
)

type ClusterConditionType string

// These are valid conditions of a cluster.
const (
	// ClusterReady means the cluster is ready to accept workloads.
	ClusterReady ClusterConditionType = "Ready"
	// ClusterOffline means the cluster is temporarily down or not reachable
	ClusterOffline ClusterConditionType = "Offline"
)
