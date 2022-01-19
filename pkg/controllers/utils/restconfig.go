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
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	multiclusterv1 "github.com/alibaba/hybridnet/apis/multicluster/v1"
)

func NewRestConfigFromRemoteCluster(remoteCluster *multiclusterv1.RemoteCluster) (*rest.Config, error) {
	clusterConfig, err := clientcmd.BuildConfigFromFlags(remoteCluster.Spec.APIEndpoint, "")
	if err != nil {
		return nil, err
	}

	clusterConfig.CAData = make([]byte, len(remoteCluster.Spec.CAData))
	copy(clusterConfig.CAData, remoteCluster.Spec.CAData)
	clusterConfig.CertData = make([]byte, len(remoteCluster.Spec.CertData))
	copy(clusterConfig.CertData, remoteCluster.Spec.CertData)
	clusterConfig.KeyData = make([]byte, len(remoteCluster.Spec.KeyData))
	copy(clusterConfig.KeyData, remoteCluster.Spec.KeyData)

	// TODO: insecure mode

	clusterConfig.Timeout = time.Second * time.Duration(remoteCluster.Spec.Timeout)
	clusterConfig.QPS = 20
	clusterConfig.Burst = 30

	return clusterConfig, nil
}
