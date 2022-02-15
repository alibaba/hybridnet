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

package validating

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	networkingv1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
)

var ipInstanceGVK = gvkConverter(networkingv1.GroupVersion.WithKind("IPInstance"))

func init() {
	createHandlers[ipInstanceGVK] = IPInstanceCreateValidation
	updateHandlers[ipInstanceGVK] = IPInstanceUpdateValidation
	deleteHandlers[ipInstanceGVK] = IPInstanceDeleteValidation
}

func IPInstanceCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	return admission.Allowed("no validation")
}

func IPInstanceUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	return admission.Allowed("no validation")
}

func IPInstanceDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	return admission.Allowed("no validation")
}
