#!/usr/bin/env bash

# Copyright 2021 The Hybridnet Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

TEMP_DIR=$(mktemp -d)
ROOT_PKG=github.com/alibaba/hybridnet

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
bash "${SCRIPT_ROOT}"/hack/generate-groups.sh "deepcopy,client,informer,lister" \
  github.com/alibaba/hybridnet/pkg/client github.com/alibaba/hybridnet/pkg/apis \
  "networking:v1 multicluster:v1" \
  --output-base "${TEMP_DIR}" \
  --go-header-file "${SCRIPT_ROOT}"/hack/custom-boilerplate.go.txt

# Copy everything back.
cp -a "${TEMP_DIR}/${ROOT_PKG}/." "${SCRIPT_ROOT}/"