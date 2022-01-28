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

package main

import (
	"flag"
	"os"

	"go.uber.org/zap/zapcore"

	"github.com/alibaba/hybridnet/pkg/feature"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/spf13/pflag"
	admissionv1 "k8s.io/api/admission/v1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/webhook/mutating"
	"github.com/alibaba/hybridnet/pkg/webhook/validating"
)

var (
	gitCommit          string
	scheme             = runtime.NewScheme()
	port               int
	metricsBindAddress string
)

func init() {
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = multiclusterv1.AddToScheme(scheme)
	_ = admissionv1beta1.AddToScheme(scheme)
	_ = admissionv1.AddToScheme(scheme)

	pflag.IntVar(&port, "port", 9898, "The port webhook listen on")
	pflag.StringVar(&metricsBindAddress, "metrics-bind-address", "0", "The bind address for metrics, eg :8080")

	ctrllog.SetLogger(zap.New(zap.UseDevMode(true), zap.Level(zapcore.InfoLevel)))
}

func main() {
	// parse flags
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	var entryLog = ctrllog.Log.WithName("entry")
	entryLog.Info("starting hybridnet manager", "known-features", feature.KnownFeatures(), "commit-id", gitCommit)

	// create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		Port:               port,
		MetricsBindAddress: metricsBindAddress,
	})
	if err != nil {
		entryLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// create webhooks
	mgr.GetWebhookServer().Register("/validate", &webhook.Admission{
		Handler: validating.NewHandler(),
	})
	mgr.GetWebhookServer().Register("/mutate", &webhook.Admission{
		Handler: mutating.NewHandler(),
	})

	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "manager exit unexpectedly")
		os.Exit(1)
	}
}
