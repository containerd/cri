#!/bin/bash

#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

source $(dirname "${BASH_SOURCE[0]}")/utils.sh

# RESTART_WAIT_PERIOD is the period to wait before restarting containerd.
RESTART_WAIT_PERIOD=${RESTART_WAIT_PERIOD:-10}
# CONTAINERD_FLAGS contains all containerd flags.
CONTAINERD_FLAGS="--log-level=debug "

# Use a configuration file for containerd.
CONTAINERD_CONFIG_FILE=${CONTAINERD_CONFIG_FILE:-""}
if [ -z "${CONTAINERD_CONFIG_FILE}" ] && command -v sestatus >/dev/null 2>&1; then
  selinux_config="/tmp/containerd-config-selinux.toml"
  cat >${selinux_config} <<<'
[plugins.cri]
  enable_selinux = true
'
  CONTAINERD_CONFIG_FILE=${CONTAINERD_CONFIG_FILE:-"${selinux_config}"}
fi

# CONTAINERD_TEST_SUFFIX is the suffix appended to the root/state directory used
# by test containerd.
CONTAINERD_TEST_SUFFIX=${CONTAINERD_TEST_SUFFIX:-"-test"}
# The containerd root directory.
CONTAINERD_ROOT=${CONTAINERD_ROOT:-"/var/lib/containerd${CONTAINERD_TEST_SUFFIX}"}
# The containerd state directory.
CONTAINERD_STATE=${CONTAINERD_STATE:-"/run/containerd${CONTAINERD_TEST_SUFFIX}"}
# The containerd socket address.
CONTAINERD_SOCK=${CONTAINERD_SOCK:-unix://${CONTAINERD_STATE}/containerd.sock}
# The containerd binary name.
CONTAINERD_BIN=${CONTAINERD_BIN:-"containerd${CONTAINERD_TEST_SUFFIX}"}
if [ -f "${CONTAINERD_CONFIG_FILE}" ]; then
  CONTAINERD_FLAGS+="--config ${CONTAINERD_CONFIG_FILE} "
fi
CONTAINERD_FLAGS+="--address ${CONTAINERD_SOCK#"unix://"} \
  --state ${CONTAINERD_STATE} \
  --root ${CONTAINERD_ROOT}"

containerd_groupid=

# test_setup starts containerd.
test_setup() {
  local report_dir=$1
  # Start containerd
  if [ ! -x "${ROOT}/_output/containerd" ]; then
    echo "containerd is not built"
    exit 1
  fi
  # rename the test containerd binary, so that we can easily
  # distinguish it.
  : GOBIN=${GOBIN:-$(go env GOBIN)}
  : GOBIN=${GOBIN:=/usr/local/bin}
  sudo mkdir -vp ${GOBIN%/}
  sudo cp -vf ${ROOT}/_output/containerd ${GOBIN%/}/${CONTAINERD_BIN}
  if type -p sestatus chcon >/dev/null 2>&1; then
    sudo chcon -v -t container_runtime_exec_t ${GOBIN%/}/${CONTAINERD_BIN}
  fi
  if (command -v getenforce >/dev/null 2>&1) && [ "$(getenforce)" = "Enforcing" ]; then
    set -e
    make -f /usr/share/selinux/devel/Makefile -C "${ROOT}/test/selinux/policy" clean containerd-test.pp
    sudo semodule -i "${ROOT}/test/selinux/policy/containerd-test.pp"
#    sudo semodule -e containerd-test
    sudo rm -fr "${CONTAINERD_ROOT}" "${CONTAINERD_STATE}"
    sudo mkdir -vp "${CONTAINERD_ROOT}" "${CONTAINERD_STATE}"
    sudo restorecon -rv "${CONTAINERD_ROOT}" "${CONTAINERD_STATE}"
    set +e
  fi
  set -m
  # Create containerd in a different process group
  # so that we can easily clean them up.
  keepalive "sudo PATH=${PATH} ${GOBIN%/}/${CONTAINERD_BIN} ${CONTAINERD_FLAGS}" \
    ${RESTART_WAIT_PERIOD} &> ${report_dir}/containerd.log &
  pid=$!
  set +m
  containerd_groupid=$(ps -o pgid= -p ${pid})
  # Wait for containerd to be running by using the containerd client ctr to check the version
  # of the containerd server. Wait an increasing amount of time after each of five attempts
  local -r ctr_path=$(which ctr)
  if [ -z "${ctr_path}" ]; then
    echo "ctr is not in PATH"
    exit 1
  fi
  local -r crictl_path=$(which crictl)
  if [ -z "${crictl_path}" ]; then
    echo "crictl is not in PATH"
    exit 1
  fi
  readiness_check "sudo ${ctr_path} --address ${CONTAINERD_SOCK#"unix://"} version"
  readiness_check "sudo ${crictl_path} --runtime-endpoint=${CONTAINERD_SOCK} info"
}

# test_teardown kills containerd.
test_teardown() {
  if [ -n "${containerd_groupid}" ]; then
    sudo pkill -g ${containerd_groupid}
  fi
}

# keepalive runs a command and keeps it alive.
# keepalive process is eventually killed in test_teardown.
keepalive() {
  local command=$1
  echo ${command}
  local wait_period=$2
  while true; do
    ${command}
    sleep ${wait_period}
  done
}

# readiness_check checks readiness of a daemon with specified command.
readiness_check() {
  local command=$1
  local MAX_ATTEMPTS=5
  local attempt_num=1
  until ${command} &> /dev/null || (( attempt_num == MAX_ATTEMPTS ))
  do
      echo "$attempt_num attempt \"$command\"! Trying again in $attempt_num seconds..."
      sleep $(( attempt_num++ ))
  done
}
