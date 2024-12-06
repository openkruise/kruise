#!/usr/bin/env bash
# Copyright (c) 2023 Alibaba Group Holding Ltd.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#      http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# Setup default values
CLUSTER_NAME=${CLUSTER_NAME:-"ci-testing"}
KIND_NODE_TAG=${KIND_NODE_TAG:-"v1.28.7"}
KIND_CONFIG=${KIND_CONFIG:-"test/kind-conf-with-vpa.yaml"}
echo "$KIND_NODE_TAG"
echo "$CLUSTER_NAME"

## Create kind cluster.
tools/bin/kind create cluster --image "kindest/node:${KIND_NODE_TAG}" --name "${CLUSTER_NAME}" --config="${KIND_CONFIG}"
