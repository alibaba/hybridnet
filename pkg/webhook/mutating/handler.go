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

package mutating

import (
	"context"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	createHandlers = make(map[metav1.GroupVersionKind]handlerFunc)
	updateHandlers = make(map[metav1.GroupVersionKind]handlerFunc)
	deleteHandlers = make(map[metav1.GroupVersionKind]handlerFunc)
)

type handlerFunc func(ctx context.Context, req *admission.Request, handler *Handler) admission.Response

type Handler struct {
	Decoder *admission.Decoder
	Cache   cache.Cache
	Client  client.Client
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case v1beta1.Create:
		if handling, exist := createHandlers[req.Kind]; exist {
			return handling(ctx, &req, h)
		}
	case v1beta1.Update:
		if handling, exist := updateHandlers[req.Kind]; exist {
			return handling(ctx, &req, h)
		}
	case v1beta1.Delete:
		if handling, exist := deleteHandlers[req.Kind]; exist {
			return handling(ctx, &req, h)
		}
	}

	return admission.Allowed("by pass")
}

func (h *Handler) InjectDecoder(decoder *admission.Decoder) error {
	h.Decoder = decoder
	return nil
}

func (h *Handler) InjectClient(client client.Client) error {
	h.Client = client
	return nil
}

func (h *Handler) InjectCache(cache cache.Cache) error {
	h.Cache = cache
	return nil
}

func gvkConverter(in schema.GroupVersionKind) metav1.GroupVersionKind {
	return metav1.GroupVersionKind{
		Group:   in.Group,
		Version: in.Version,
		Kind:    in.Kind,
	}
}
