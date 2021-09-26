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

package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	networkingv1 "github.com/oecp/rama/pkg/apis/networking/v1"
	"github.com/oecp/rama/pkg/constants"
)

const (
	KubeAPIQPS   = 20.0
	KubeAPIBurst = 30
)

func BuildClusterConfig(rc *networkingv1.RemoteCluster) (*restclient.Config, error) {
	var (
		err         error
		clusterName = rc.ClusterName
		connConfig  = rc.Spec.ConnConfig
	)
	clusterConfig, err := clientcmd.BuildConfigFromFlags(connConfig.Endpoint, "")
	if err != nil {
		return nil, err
	}

	if len(connConfig.ClientCert) == 0 || len(connConfig.CABundle) == 0 || len(connConfig.ClientKey) == 0 {
		return nil, errors.Errorf("The connection data for cluster %s is missing", clusterName)
	}

	clusterConfig.Timeout = time.Duration(connConfig.Timeout) * time.Second
	clusterConfig.CAData = connConfig.CABundle
	clusterConfig.CertData = connConfig.ClientCert
	clusterConfig.KeyData = connConfig.ClientKey
	clusterConfig.QPS = KubeAPIQPS
	clusterConfig.Burst = KubeAPIBurst

	return clusterConfig, nil
}

func SelectorClusterName(clusterName string) labels.Selector {
	s := labels.Set{
		constants.LabelCluster: clusterName,
	}
	return labels.SelectorFromSet(s)
}

func GetUUID(client kubernetes.Interface) (types.UID, error) {
	ns, err := client.CoreV1().Namespaces().Get(context.TODO(), "kube-system", metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Can't get uuid. err=%v", err)
		return "", err
	}
	return ns.UID, nil
}

func GetUUIDFromRemoteCluster(rc *networkingv1.RemoteCluster) (types.UID, error) {
	cfg, err := BuildClusterConfig(rc)
	if err != nil {
		return "", err
	}

	k8sClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("invalid config for building client: %v", err)
	}

	return GetUUID(k8sClient)
}
