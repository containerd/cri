#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
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
set -o nounset

cd "$( dirname "${BASH_SOURCE[0]}" )"

# Use eval to preserve embedded quoted strings.
eval "BUILD_TAGS=(${1:-})"
eval "GO_LDFLAGS=(${2:-})"
eval "GO_GCFLAGS=(${3:-})"

# Check GOPATH
if [[ -z "${GOPATH}" ]]; then
  echo "GOPATH is not set"
  exit 1
fi

GO=$(which go)
sudo env GOPATH=${GOPATH} ${GO} test -timeout=10m -race ./../pkg/... \
     "${BUILD_TAGS[@]:+${BUILD_TAGS[@]}}" \
     "${GO_LDFLAGS[@]:+${GO_LDFLAGS[@]}}" \
     "${GO_GCFLAGS[@]:+${GO_GCFLAGS[@]}}"
