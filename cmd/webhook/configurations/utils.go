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

package configurations

import (
	"fmt"
	"strings"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/apis/admissionregistration"

	v1 "github.com/oecp/rama/cmd/webhook/configurations/v1"
	"github.com/oecp/rama/cmd/webhook/configurations/v1beta1"
)

type APIVersion string

const (
	arv1      = APIVersion("v1")
	arv1beta1 = APIVersion("v1beta1")
)

func FetchAPIVersion(cfg *rest.Config) (APIVersion, error) {
	c := discovery.NewDiscoveryClientForConfigOrDie(cfg)

	apiGroupList, err := c.ServerGroups()
	if err != nil {
		return "", err
	}

	for _, ag := range apiGroupList.Groups {
		if ag.Name == admissionregistration.GroupName {
			switch strings.ToLower(ag.PreferredVersion.Version) {
			case "v1":
				return arv1, nil
			case "v1beta1":
				return arv1beta1, nil
			default:
				return arv1beta1, nil
			}
		}
	}

	return "", fmt.Errorf("api group %s not found", admissionregistration.GroupName)
}

func BuildConfigurations(version APIVersion, address string, port int, caCertPath string) {
	switch version {
	case arv1:
		v1.BuildConfigurations(address, port, caCertPath)
	case arv1beta1:
		fallthrough
	default:
		v1beta1.BuildConfigurations(address, port, caCertPath)
	}
}

func EnsureValidatingWebhookConfiguration(version APIVersion, cfg *rest.Config) {
	switch version {
	case arv1:
		v1.EnsureValidatingWebhookConfiguration(cfg)
	case arv1beta1:
		fallthrough
	default:
		v1beta1.EnsureValidatingWebhookConfiguration(cfg)
	}
}

func EnsureMutatingWebhookConfiguration(version APIVersion, cfg *rest.Config) {
	switch version {
	case arv1:
		v1.EnsureMutatingWebhookConfiguration(cfg)
	case arv1beta1:
		fallthrough
	default:
		v1beta1.EnsureMutatingWebhookConfiguration(cfg)
	}
}
