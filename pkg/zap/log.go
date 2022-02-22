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

package zap

import (
	"flag"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var zapOptions zap.Options

func init() {
	zapOptions.BindFlags(flag.CommandLine)
}

// NewZapLogger should be called after parsing flags
func NewZapLogger() logr.Logger {
	var opts = []zap.Opts{
		zap.UseFlagOptions(&zapOptions),
	}

	// default encoder to console mode
	if zapOptions.NewEncoder == nil {
		opts = append(opts, zap.ConsoleEncoder())
	}
	return zap.New(opts...)
}
