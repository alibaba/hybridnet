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

package v1beta1

import (
	"context"
	"fmt"
	"io/ioutil"

	arv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	arclient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/oecp/rama/cmd/webhook/configurations/constants"
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
)

var validatingWebhookConfiguration *arv1beta1.ValidatingWebhookConfiguration
var mutatingWebhookConfiguration *arv1beta1.MutatingWebhookConfiguration

func BuildConfigurations(address string, port int, caCertPath string) {
	validatingWebhookConfiguration = &arv1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.ValidatingWebhookName,
		},
		Webhooks: []arv1beta1.ValidatingWebhook{
			{
				Name: "rama-v1.validating.rama",
				ClientConfig: arv1beta1.WebhookClientConfig{
					URL:      stringPointer(fmt.Sprintf("https://%s:%d/validate", address, port)),
					CABundle: getCABundle(caCertPath),
				},
				ObjectSelector: generateObjectSelector(),
				Rules: []arv1beta1.RuleWithOperations{
					{
						Operations: []arv1beta1.OperationType{
							arv1beta1.Create,
							arv1beta1.Delete,
							arv1beta1.Update,
						},
						Rule: arv1beta1.Rule{
							APIGroups: []string{
								ramav1.SchemeGroupVersion.Group,
							},
							APIVersions: []string{
								ramav1.SchemeGroupVersion.Version,
							},
							Resources: []string{
								"networks",
								"subnets",
							},
						},
					},
				},
				FailurePolicy:           failurePolicyPointer(arv1beta1.Fail),
				SideEffects:             sideEffectClassPointer(arv1beta1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1beta1"},
			},
			{
				Name: "core-v1.validating.rama",
				ClientConfig: arv1beta1.WebhookClientConfig{
					URL:      stringPointer(fmt.Sprintf("https://%s:%d/validate", address, port)),
					CABundle: getCABundle(caCertPath),
				},
				ObjectSelector: generateObjectSelector(),
				Rules: []arv1beta1.RuleWithOperations{
					{
						Operations: []arv1beta1.OperationType{
							arv1beta1.Create,
						},
						Rule: arv1beta1.Rule{
							APIGroups: []string{
								corev1.SchemeGroupVersion.Group,
							},
							APIVersions: []string{
								corev1.SchemeGroupVersion.Version,
							},
							Resources: []string{
								"pods",
							},
						},
					},
				},
				FailurePolicy:           failurePolicyPointer(arv1beta1.Fail),
				SideEffects:             sideEffectClassPointer(arv1beta1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1beta1"},
			},
		},
	}

	mutatingWebhookConfiguration = &arv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.MutatingWebhookName,
		},
		Webhooks: []arv1beta1.MutatingWebhook{
			{
				Name: "core-v1.mutating.rama",
				ClientConfig: arv1beta1.WebhookClientConfig{
					URL:      stringPointer(fmt.Sprintf("https://%s:%d/mutate", address, port)),
					CABundle: getCABundle(caCertPath),
				},
				ObjectSelector: generateObjectSelector(),
				Rules: []arv1beta1.RuleWithOperations{
					{
						Operations: []arv1beta1.OperationType{
							arv1beta1.Create,
						},
						Rule: arv1beta1.Rule{
							APIGroups: []string{
								corev1.SchemeGroupVersion.Group,
							},
							APIVersions: []string{
								corev1.SchemeGroupVersion.Version,
							},
							Resources: []string{
								"pods",
							},
						},
					},
				},
				FailurePolicy:           failurePolicyPointer(arv1beta1.Fail),
				SideEffects:             sideEffectClassPointer(arv1beta1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1beta1"},
			},
		},
	}
}

func EnsureValidatingWebhookConfiguration(cfg *rest.Config) {
	c := arclient.NewForConfigOrDie(cfg)

	var err error
	if _, err = c.ValidatingWebhookConfigurations().Get(context.TODO(), constants.ValidatingWebhookName, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) {
			if _, err = c.ValidatingWebhookConfigurations().Create(context.TODO(), validatingWebhookConfiguration, metav1.CreateOptions{}); err != nil {
				klog.Fatalf("fail to create validating webhook configuration: %v", err)
			}
			return
		}
		klog.Fatalf("fail to get validating webhook configuratoin: %v", err)
	}

	_, _ = c.ValidatingWebhookConfigurations().Update(context.TODO(), validatingWebhookConfiguration, metav1.UpdateOptions{})
	return
}

func EnsureMutatingWebhookConfiguration(cfg *rest.Config) {
	var err error

	c := arclient.NewForConfigOrDie(cfg)
	if _, err = c.MutatingWebhookConfigurations().Get(context.TODO(), constants.MutatingWebhookName, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) {
			if _, err = c.MutatingWebhookConfigurations().Create(context.TODO(), mutatingWebhookConfiguration, metav1.CreateOptions{}); err != nil {
				klog.Fatalf("fail to create mutating webhook configuration: %v", err)
			}
			return
		}
		klog.Fatalf("fail to get mutating webhook configuratoin: %v", err)
	}

	_, _ = c.MutatingWebhookConfigurations().Update(context.TODO(), mutatingWebhookConfiguration, metav1.UpdateOptions{})
	return
}

func stringPointer(s string) *string {
	return &s
}

func failurePolicyPointer(f arv1beta1.FailurePolicyType) *arv1beta1.FailurePolicyType {
	return &f
}

func sideEffectClassPointer(s arv1beta1.SideEffectClass) *arv1beta1.SideEffectClass {
	return &s
}

func getCABundle(caCertPath string) []byte {
	content, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		klog.Fatalf("ca bundle file not found: %v", err)
	}
	return content
}

func generateObjectSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      constants.LabelWebhookIgnore,
				Operator: metav1.LabelSelectorOpNotIn,
				Values: []string{
					"TRUE",
					"true",
				},
			},
		},
	}
}
