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

if [ -z "${PKG}" ]; then
  echo "PKG must be set"
  exit 1
fi

# enabled to use host dns resolver
export CGO_ENABLED=1
export GODEBUG=netdns=cgo+2
# use vendor directory instead of go modules https://github.com/golang/go/wiki/Modules
export GO111MODULE=off

go test -v \
  $(go list "${PKG}/..." | grep -v vendor | grep -v '/test/e2e' | grep -v images | grep -v "docs/examples")
