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

package bgp

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/osrg/gobgp/v3/pkg/log"
)

// implement github.com/osrg/gobgp/v3/pkg/log/Logger interface
type bgpLogger struct {
	logger logr.Logger
}

func (l *bgpLogger) Panic(msg string, fields log.Fields) {
	l.logger.Error(fmt.Errorf("%v", msg), "bgp panic log")
	os.Exit(1)
}

func (l *bgpLogger) Fatal(msg string, fields log.Fields) {
	l.logger.Error(fmt.Errorf("%v", msg), "bgp fatal log")
	os.Exit(1)
}

func (l *bgpLogger) Error(msg string, fields log.Fields) {
	l.logger.Error(fmt.Errorf("%v", msg), "bgp error log")
}

func (l *bgpLogger) Warn(msg string, fields log.Fields) {
	l.logger.Info("bgp warn log", "message", msg)
}

func (l *bgpLogger) Info(msg string, fields log.Fields) {
	l.logger.Info("bgp info log", "message", msg)
}

func (l *bgpLogger) Debug(msg string, fields log.Fields) {
	l.logger.V(2).Info("bgp debug log", "message", msg)
}

func (l *bgpLogger) SetLevel(level log.LogLevel) {
}

func (l *bgpLogger) GetLevel() log.LogLevel {
	return log.FatalLevel
}
