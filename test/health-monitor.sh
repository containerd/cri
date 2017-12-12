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
set -o pipefail

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"/..
source ${ROOT}/hack/test-utils.sh

function containerd_monitoring {
  while [ 1 ]; do
    if ! timeout 60 ctr containers ls > /dev/null; then
      echo "Containerd failed!"
      pkill containerd
      # Wait and check, make sure containerd is really up.
      readiness_check "timeout 60 ctr containers ls > /dev/null"
    else
      sleep "${SLEEP_SECOMDS}"
    fi
  done
}

function cri_containerd_monitoring {
  while [ 1 ]; do
    if ! timeout 60 crictl --runtime-endpoint /var/run/cri-containerd.sock ps > /dev/null; then
      echo "cri-containerd failed!"
      pkill cri-containerd
      # Wait and check, make sure cri-containerd is really up.
      readiness_check "timeout 60 crictl --runtime-endpoint /var/run/cri-containerd.sock ps > /dev/null"
    else
      sleep "${SLEEP_SECOMDS}"
    fi
  done
}

############## Main Function ################
if [[ "$#" -ne 1 ]]; then
  echo "Usage: health-monitor.sh <cri-containerd/containerd>"
  exit 1
fi

SLEEP_SECONDS=10
component=$1
echo "Start kubernetes health monitoring for ${component}"
if [[ "${component}" == "containerd" ]]; then
  containerd_monitoring
elif [[ "${component}" == "cri-containerd" ]]; then
  cri_containerd_monitoring
else
  echo "Health monitoring for component "${component}" is not supported!"
fi
