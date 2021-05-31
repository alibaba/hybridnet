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

package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/oecp/rama/pkg/daemon/config"
	"github.com/oecp/rama/pkg/daemon/contorller"
	"github.com/oecp/rama/pkg/request"

	"github.com/emicklei/go-restful"

	"k8s.io/klog"
)

var requestLogString = "[%s] Incoming %s %s %s request"
var responseLogString = "[%s] Outcoming response %s %s with %d status code in %vms"

// RunServer runs the cniDaemon http restful server
func RunServer(stopCh <-chan struct{}, config *config.Configuration, ctrlRef *contorller.Controller) {
	cdh, err := createCniDaemonHandler(stopCh, config, ctrlRef)
	if err != nil {
		klog.Errorf("create cni daemon handler with socket %v failed: %v", config.BindSocket, err)
		return
	}
	server := http.Server{
		Handler: createHandler(cdh),
	}
	unixListener, err := net.Listen("unix", config.BindSocket)
	if err != nil {
		klog.Errorf("bind socket to %s failed %v", config.BindSocket, err)
		return
	}
	defer os.Remove(config.BindSocket)
	klog.Infof("start listen on %s", config.BindSocket)
	klog.Fatal(server.Serve(unixListener))
}

func createHandler(cdh *cniDaemonHandler) http.Handler {
	wsContainer := restful.NewContainer()
	wsContainer.EnableContentEncoding(true)

	ws := new(restful.WebService)
	ws.Path("/api/v1").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)
	wsContainer.Add(ws)

	ws.Route(
		ws.POST("/add").
			To(cdh.handleAdd).
			Reads(request.PodRequest{}))
	ws.Route(
		ws.POST("/del").
			To(cdh.handleDel).
			Reads(request.PodRequest{}))

	ws.Filter(requestAndResponseLogger)

	return wsContainer
}

// web-service filter function used for request and response logging.
func requestAndResponseLogger(request *restful.Request, response *restful.Response,
	chain *restful.FilterChain) {
	klog.Infof(formatRequestLog(request))
	start := time.Now()
	chain.ProcessFilter(request, response)
	elapsed := float64((time.Since(start)) / time.Millisecond)
	klog.Infof(formatResponseLog(response, request, elapsed))
}

// formatRequestLog formats request log string.
func formatRequestLog(request *restful.Request) string {
	uri := ""
	if request.Request.URL != nil {
		uri = request.Request.URL.RequestURI()
	}

	return fmt.Sprintf(requestLogString, time.Now().Format(time.RFC3339), request.Request.Proto,
		request.Request.Method, uri)
}

// formatResponseLog formats response log string.
func formatResponseLog(response *restful.Response, request *restful.Request, reqTime float64) string {
	uri := ""
	if request.Request.URL != nil {
		uri = request.Request.URL.RequestURI()
	}
	return fmt.Sprintf(responseLogString, time.Now().Format(time.RFC3339),
		request.Request.Method, uri, response.StatusCode(), reqTime)
}
