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

package server

import (
	"context"
	"net"
	"net/http"
	"os"

	"github.com/go-logr/logr"

	"github.com/alibaba/hybridnet/pkg/daemon/config"
	"github.com/alibaba/hybridnet/pkg/daemon/controller"
	"github.com/alibaba/hybridnet/pkg/request"

	"github.com/emicklei/go-restful"
)

// RunServer runs the cniDaemon http restful server
func RunServer(ctx context.Context, config *config.Configuration, ctrlRef *controller.CtrlHub, logger logr.Logger) {
	cdh, err := createCniDaemonHandler(ctx, config, ctrlRef, logger.WithName("daemon-cni-server"))
	if err != nil {
		logger.Error(err, "failed to create cni daemon handler", "socket path", config.BindSocket)
		return
	}
	server := http.Server{
		Handler: createHandler(cdh),
	}
	unixListener, err := net.Listen("unix", config.BindSocket)
	if err != nil {
		logger.Error(err, "failed to bind socket", "socket path", config.BindSocket)
		return
	}
	defer os.Remove(config.BindSocket)
	logger.Info("server started", "socket path", config.BindSocket)

	err = server.Serve(unixListener)
	logger.Error(err, "server exist unexpected")
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

	return wsContainer
}
