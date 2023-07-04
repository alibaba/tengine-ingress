#!/bin/bash

# Copyright 2019 The Kubernetes Authors.
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

set -e

NC='\e[0m'
BGREEN='\e[32m'

SLOW_E2E_THRESHOLD=${SLOW_E2E_THRESHOLD:-50}
FOCUS=${FOCUS:-.*}
E2E_NODES=${E2E_NODES:-5}
E2E_CHECK_LEAKS=${E2E_CHECK_LEAKS:-""}

if [ ! -f "${HOME}/.kube/config" ]; then
  kubectl config set-cluster dev --certificate-authority=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt --embed-certs=true --server="https://kubernetes.default/"
  kubectl config set-credentials user --token="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"
  kubectl config set-context default --cluster=dev --user=user
  kubectl config use-context default
fi

ginkgo_args=(
  "-randomizeSuites"
  "-randomizeAllSpecs"
  "-flakeAttempts=2"
  "-p"
  "-trace"
  "-slowSpecThreshold=${SLOW_E2E_THRESHOLD}"
  "-r"
  "-timeout=45m" # Suite timeout should be lower than Prow job timeout to avoid abrupt termination
)

echo -e "${BGREEN}Running e2e test suite (FOCUS=${FOCUS})...${NC}"
ginkgo "${ginkgo_args[@]}"               \
  -focus="${FOCUS}"                      \
  -skip="\[Serial\]|\[MemoryLeak\]"      \
  -nodes="${E2E_NODES}"                  \
  /e2e.test

echo -e "${BGREEN}Running e2e test suite with tests that require serial execution...${NC}"
ginkgo "${ginkgo_args[@]}"               \
  -focus="\[Serial\]"                    \
  -skip="\[MemoryLeak\]"                 \
  -nodes=1                               \
  /e2e.test

if [[ ${E2E_CHECK_LEAKS} != "" ]]; then
  echo -e "${BGREEN}Running e2e test suite with tests that check for memory leaks...${NC}"
  ginkgo "${ginkgo_args[@]}"             \
    -focus="\[MemoryLeak\]"              \
    -skip="\[Serial\]"                   \
    -nodes=1                             \
    /e2e.test
fi
