#!/bin/bash

# Copyright 2018 The Kubernetes Authors.
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

if [ -n "$DEBUG" ]; then
	set -x
fi

set -o errexit
set -o nounset
set -o pipefail

export GOPROXY="https://goproxy.io,direct"

export PKG=k8s.io/ingress-nginx
export ARCH=$(go env GOARCH)
export TAG="1.0.0"
export GIT_COMMIT=git-$(git rev-parse --short HEAD)
export REPO_INFO=$(git config --get remote.origin.url)
export GOBUILD_FLAGS="-v"

export CGO_ENABLED=1
export GOARCH=${ARCH}

#  -ldflags "-s -w \
go build \
  -mod vendor \
  "${GOBUILD_FLAGS}" \
  -gcflags "all=-N -l" \
  -ldflags " \
    -X ${PKG}/version.RELEASE=${TAG} \
    -X ${PKG}/version.COMMIT=${GIT_COMMIT} \
    -X ${PKG}/version.REPO=${REPO_INFO}" \
  -o "bin/tengine-ingress-controller" "${PKG}/cmd/nginx"

go build \
  -mod vendor \
  "${GOBUILD_FLAGS}" \
  -ldflags "-s -w \
    -X ${PKG}/version.RELEASE=${TAG} \
    -X ${PKG}/version.COMMIT=${GIT_COMMIT} \
    -X ${PKG}/version.REPO=${REPO_INFO}" \
  -o "bin/dbg" "${PKG}/cmd/dbg"


go build \
  -mod vendor \
  "${GOBUILD_FLAGS}" \
  -ldflags "-s -w \
    -X ${PKG}/version.RELEASE=${TAG} \
    -X ${PKG}/version.COMMIT=${GIT_COMMIT} \
    -X ${PKG}/version.REPO=${REPO_INFO}" \
  -o "bin/wait-shutdown" "${PKG}/cmd/waitshutdown"
