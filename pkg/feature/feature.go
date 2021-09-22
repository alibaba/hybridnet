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

package feature

import (
	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog"
)

func init() {
	if err := feature.DefaultMutableFeatureGate.Add(DefaultRamaFeatureGates); err != nil {
		klog.Fatalf("feature gate init fails: %v", err)
	}

	feature.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)
}

const (
	// owner: @bruce.mwj
	// alpha: v0.1
	//
	// Enable dual stack allocation in IPAM.
	DualStack featuregate.Feature = "DualStack"

	MultiCluster featuregate.Feature = "MultiCluster"
)

var DefaultRamaFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	DualStack: {
		Default:    false,
		PreRelease: featuregate.Alpha,
	},
	MultiCluster: {
		Default:    false,
		PreRelease: featuregate.Alpha,
	},
}

func DualStackEnabled() bool {
	return feature.DefaultMutableFeatureGate.Enabled(DualStack)
}

func MultiClusterEnabled() bool {
	return feature.DefaultMutableFeatureGate.Enabled(MultiCluster)
}

func KnownFeatures() []string {
	return feature.DefaultMutableFeatureGate.KnownFeatures()
}
