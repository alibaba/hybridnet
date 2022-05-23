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

package networking_test

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/constants"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("IP allocation test", func() {
	const timeout = time.Second * 5
	const interval = time.Microsecond * 500

	It("IP allocation for single Pod", func() {
		By("Create a stateful Pod and wait for IPInstance")
		ctx := context.Background()
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node1",
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "test",
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, pod1)).Should(Succeed())
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{
			Name:      "test-pod",
			Namespace: "default",
		}, pod1)).Should(Succeed())

		Eventually(func() error {
			ipInstanceList := &networkingv1.IPInstanceList{}
			if err := k8sClient.List(context.TODO(), ipInstanceList, client.MatchingLabels{
				constants.LabelNode: pod1.Spec.NodeName,
				constants.LabelPod:  pod1.Name,
			}); err != nil {
				return err
			}

			if len(ipInstanceList.Items) == 0 {
				return fmt.Errorf("no ip instance allocated for pod %v", pod1.Name)
			}

			if ipInstanceList.Items[0].Spec.Binding.PodUID != pod1.GetUID() ||
				ipInstanceList.Items[0].Spec.Binding.ReferredObject.UID != pod1.GetUID() {
				return fmt.Errorf("error ip instance pod uid %v, expected %v",
					ipInstanceList.Items[0].Spec.Binding.PodUID, pod1.GetUID())
			}

			return nil
		}, timeout, interval).Should(Succeed())
	})
})
