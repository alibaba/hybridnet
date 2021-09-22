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

package manager

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/oecp/rama/pkg/client/clientset/versioned"
	"github.com/oecp/rama/pkg/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	clientconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	Name      = "rama-manager"
	Namespace = "kube-system"

	LeaderElectionUserAgent = "leader-election"
	RamaUserAgent           = "rama"
	recorderUserAgent       = "rama-recorder"

	DefaultLeaseDuration = 15 * time.Second
	DefaultRenewDeadline = 10 * time.Second
	DefaultRetryPeriod   = 2 * time.Second
)

type Manager struct {
	KubeConfig *rest.Config

	KubeClient           kubernetes.Interface
	RamaClient           versioned.Interface
	LeaderElectionClient kubernetes.Interface

	InformerFactory     informers.SharedInformerFactory
	RamaInformerFactory externalversions.SharedInformerFactory

	recorder record.EventRecorder

	StopEverything <-chan struct{}
}

func NewManager() (*Manager, error) {
	config, err := clientconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("fail to get kubernetes config")
	}

	kubeClient := kubernetes.NewForConfigOrDie(config)
	ramaClient := versioned.NewForConfigOrDie(rest.AddUserAgent(config, RamaUserAgent))

	// build shared informer factory
	informerFactory := informers.NewSharedInformerFactory(kubeClient, 0)

	// rama shared informer factory
	ramaInformerFactory := externalversions.NewSharedInformerFactory(ramaClient, 0)

	// build leader election client set
	leaderElectionClient := kubernetes.NewForConfigOrDie(rest.AddUserAgent(config, LeaderElectionUserAgent))

	// build event recorder
	recorderClient := kubernetes.NewForConfigOrDie(rest.AddUserAgent(config, recorderUserAgent))

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1.EventSinkImpl{Interface: recorderClient.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: Name})

	m := &Manager{
		KubeConfig:           config,
		KubeClient:           kubeClient,
		RamaClient:           ramaClient,
		LeaderElectionClient: leaderElectionClient,
		InformerFactory:      informerFactory,
		RamaInformerFactory:  ramaInformerFactory,
		recorder:             eventRecorder,
	}

	err = initControllers(m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) RunOrDie(ctx context.Context) error {
	id, err := os.Hostname()
	if err != nil {
		return err
	}

	// add random key to resource lock ID
	identity := fmt.Sprintf("%s-%s", id, rand.String(10))

	lock := &resourcelock.ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{
			Namespace: Namespace,
			Name:      Name,
		},
		Client: m.LeaderElectionClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      identity,
			EventRecorder: m.recorder,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: 30 * time.Second,
		RenewDeadline: 15 * time.Second,
		RetryPeriod:   5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: m.Run,
			OnStoppedLeading: func() {
				klog.Fatalf("leader election lost")
			},
		},
	})

	panic("unreachable")
}

func (m *Manager) Run(ctx context.Context) {
	defer runtime.HandleCrash()

	m.StopEverything = ctx.Done()

	klog.Info("Running controllers")
	err := runControllers(m)
	if err != nil {
		klog.Fatal(err)
	}

	// informer factory must be started after controller initializations
	klog.Info("Starting shared informer factory")
	go m.InformerFactory.Start(m.StopEverything)
	klog.Info("Starting rama shared informer factory")
	go m.RamaInformerFactory.Start(m.StopEverything)

	klog.Info("Started rama manager")
	<-m.StopEverything
	klog.Info("Shutting down rama manager")
}
