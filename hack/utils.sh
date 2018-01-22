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

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"/..

RUNC_PKG=github.com/opencontainers/runc
CNI_PKG=github.com/containernetworking/plugins
CONTAINERD_PKG=github.com/containerd/containerd
CRITOOL_PKG=github.com/kubernetes-incubator/cri-tools
CRI_CONTAINERD_PKG=github.com/containerd/cri-containerd
KUBERNETES_PKG=k8s.io/kubernetes

# upload_logs_to_gcs uploads test logs to gcs.
# Var set:
# 1. Bucket: gcs bucket to upload logs.
# 2. Dir: directory name to upload logs.
# 3. Test Result: directory of the test result.
upload_logs_to_gcs() {
  local -r bucket=$1
  local -r dir=$2
  local -r result=$3
  if ! gsutil ls "gs://${bucket}" > /dev/null; then
    create_ttl_bucket ${bucket}
  fi
  local -r upload_log_path=${bucket}/${dir}
  gsutil cp -r "${result}" "gs://${upload_log_path}"
  echo "Test logs are uploaed to:
    http://gcsweb.k8s.io/gcs/${upload_log_path}/"
}

# create_ttl_bucket create a public bucket in which all objects
# have a default TTL (30 days).
# Var set:
# 1. Bucket: gcs bucket name.
create_ttl_bucket() {
  local -r bucket=$1
  gsutil mb "gs://${bucket}"
  local -r bucket_rule=$(mktemp)
  # Set 30 day TTL for logs inside the bucket.
  echo '{"rule": [{"action": {"type": "Delete"},"condition": {"age": 30}}]}' > ${bucket_rule}
  gsutil lifecycle set "${bucket_rule}" "gs://${bucket}"
  rm "${bucket_rule}"

  gsutil -m acl ch -g all:R "gs://${bucket}"
  gsutil defacl set public-read "gs://${bucket}"
}

# sha256 generates a sha256 checksum for a file.
# Var set:
# 1. Filename.
sha256() {
  if which sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  else
    shasum -a256 "$1" | awk '{ print $1 }'
  fi
}

# checkout_repo checks out specified repository
# and switch to specified  version.
# Var set:
# 1) Pkg name.
# 2) Version.
# 3) Repo name (optional).
# Env set:
# 1) GOPATH
checkout_repo() {
  local -r pkg=$1
  local -r version=$2
  local repo=${3:-""}
  if [ -z "${repo}" ]; then
    repo=${pkg}
  fi
  if [[ -z "${GOPATH}" ]]; then
    echo "GOPATH is not set"
    exit 1
  fi
  path="${GOPATH}/src/${pkg}"
  if [ ! -d ${path} ]; then
    mkdir -p ${path}
    git clone https://${repo} ${path}
  fi
  cd ${path}
  git fetch --all
  git checkout ${version}
  cd -
}

# remove_cri_plugin removes cri plugin from containerd repository.
# Var set:
# 1. Repo: containerd repo path.
remove_cri_plugin() {
  local -r repo=$1
  cd ${repo}
  # 1. Remove cri-containerd dependency
  local -r escaped_cri_containerd_pkg=$(echo ${CRI_CONTAINERD_PKG} | sed 's/\//\\\//g')
  sed -i "/${escaped_cri_containerd_pkg}/d" cmd/containerd/builtins_linux.go
  # 2. Remove unused dependencies from vendor.conf
  if [ ! -x "$(command -v vndr)" ]; then
    echo "vndr is not installed"
    exit 1
  fi
  vndr > /dev/null 2>&1
  tmp_vendor=$(mktemp /tmp/vendor.conf.XXXX)
  while read vendor; do
    # Another possible option is to reply on `vndr` output to check
    # which vendor is unused, and remove it from vendor.conf.
    # However, `vndr` doesn't report unused if the unused vendor contains
    # LICENSE, vendor.conf etc, so we can't reply on that.
    # Here, we assume that if a vendor doesn't contain go source files
    # after `vndr`, it can be removed from vendor.conf.
    vrepo=$(echo ${vendor} | awk '{print $1}')
    if [ ! -d "vendor/${vrepo}" ]; then
      continue
    fi
    gofiles=$(find "vendor/${vrepo}" -type f -name "*.go")
    if [ -z "$gofiles" ]; then
      continue
    fi
    echo ${vendor} >> ${tmp_vendor}
  done < vendor.conf
  mv ${tmp_vendor} vendor.conf
  cd -
}
