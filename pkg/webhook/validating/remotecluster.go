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
	"net/http"
	"regexp"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	multiclusterv1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
)

var (
	rcLock           sync.Mutex
	validEndpoint    = regexp.MustCompile(`^(https?://)[\w-]+(\.[\w-]+)+:\d{1,5}$`)
	remoteClusterGVK = gvkConverter(multiclusterv1.GroupVersion.WithKind("RemoteCluster"))
)

func init() {
	createHandlers[remoteClusterGVK] = RCCreateValidation
	updateHandlers[remoteClusterGVK] = RCUpdateValidation
	deleteHandlers[remoteClusterGVK] = RCDeleteValidation
}

func RCCreateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	rc := &multiclusterv1.RemoteCluster{}
	if err := handler.Decoder.Decode(*req, rc); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	return validate(ctx, rc, handler)
}

func RCUpdateValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	newRC := &multiclusterv1.RemoteCluster{}
	if err := handler.Decoder.DecodeRaw(req.Object, newRC); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	return validate(ctx, newRC, handler)
}

func RCDeleteValidation(ctx context.Context, req *admission.Request, handler *Handler) admission.Response {
	return admission.Allowed("validation pass")
}

func validate(ctx context.Context, rc *multiclusterv1.RemoteCluster, handler *Handler) admission.Response {
	rcLock.Lock()
	defer rcLock.Unlock()

	// validate connection config
	if rc.Spec.APIEndpoint == "" {
		return admission.Denied("invalid empty endpoint")
	}
	if len(rc.Spec.CAData) == 0 || len(rc.Spec.CertData) == 0 || len(rc.Spec.KeyData) == 0 {
		return admission.Denied("invalid empty certificate info")
	}

	// validate endpoint format
	if !validEndpoint.Match([]byte(rc.Spec.APIEndpoint)) {
		return admission.Denied("endpoint format: https://server:address, please check")
	}

	return admission.Allowed("validation pass")
}
