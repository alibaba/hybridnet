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

type ClusterStatus string

const (
	ClusterReady    = ClusterStatus("Ready")
	ClusterNotReady = ClusterStatus("NotReady")
	ClusterOffline  = ClusterStatus("Offline")
	ClusterUnknown  = ClusterStatus("Unknown")
)

type ClusterConditionType string
