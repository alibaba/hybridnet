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

package main

import (
	"flag"

	"github.com/spf13/pflag"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/oecp/rama/cmd/webhook/configurations"
	ramav1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/feature"
	"github.com/oecp/rama/pkg/webhook/mutating"
	"github.com/oecp/rama/pkg/webhook/validating"
)

var (
	scheme             = runtime.NewScheme()
	port               int
	address            string
	metricsBindAddress string
	caCertPath         string
)

func init() {
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = ramav1.AddToScheme(scheme)
	_ = admissionv1beta1.AddToScheme(scheme)

	pflag.IntVar(&port, "port", 9898, "The port webhook listen on")
	pflag.StringVar(&address, "address", "127.0.0.1", "The address webhook listen with")
	pflag.StringVar(&metricsBindAddress, "metrics-bind-address", "0", "The bind address for metrics, eg :8080")
	pflag.StringVar(&caCertPath, "ca-cert-path", "/tmp/k8s-webhook-server/serving-certs/ca.crt", "The root CA certificate for apiserver to invoke webhooks")

	klog.InitFlags(nil)

	// controller-runtime initialize logger with fake implementation
	// so we should new another Logger for this
	ctrl.SetLogger(klogr.New())
}

func main() {
	// parse flags
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	klog.Infof("known features: %v", feature.KnownFeatures())

	// create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		Port:               port,
		MetricsBindAddress: metricsBindAddress,
	})
	if err != nil {
		klog.Fatal(err)
	}

	apiVersion, err := configurations.FetchAPIVersion(mgr.GetConfig())
	if err != nil {
		klog.Fatalf("fail to fetch api version: %v", err)
	}
	klog.Infof("prefer api version %s", apiVersion)

	configurations.BuildConfigurations(apiVersion, address, port, caCertPath)

	go configurations.EnsureValidatingWebhookConfiguration(apiVersion, mgr.GetConfig())
	go configurations.EnsureMutatingWebhookConfiguration(apiVersion, mgr.GetConfig())

	// create webhooks
	mgr.GetWebhookServer().Register("/validate", &webhook.Admission{
		Handler: validating.NewHandler(),
	})
	mgr.GetWebhookServer().Register("/mutate", &webhook.Admission{
		Handler: mutating.NewHandler(),
	})

	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatal(err)
	}
}
