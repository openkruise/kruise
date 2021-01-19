#!/usr/bin/env bash

# Copyright 2021 The Kruise Authors.
# Copyright 2016 The Kubernetes Authors.
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
set -o pipefail
set -o nounset

E2E_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
export E2E_ROOT
echo "$E2E_ROOT"

# Install tools we need, but only from vendor/...
go get github.com/go-bindata/go-bindata/go-bindata

# run the generation from the root directory for stable output
pushd "${E2E_ROOT}" >/dev/null

# These are files for e2e tests.
BINDATA_OUTPUT="test/e2e/generated/bindata.go"
go-bindata -nometadata -o "${BINDATA_OUTPUT}.tmp" -pkg generated \
  "test/e2e/testing-manifests/..."

gofmt -s -w "${BINDATA_OUTPUT}.tmp"

# Here we compare and overwrite only if different to avoid updating the
# timestamp and triggering a rebuild. The 'cat' redirect trick to preserve file
# permissions of the target file.
if ! cmp -s "${BINDATA_OUTPUT}.tmp" "${BINDATA_OUTPUT}" ; then
	cat "${BINDATA_OUTPUT}.tmp" > "${BINDATA_OUTPUT}"
	echo "Generated bindata file : ${BINDATA_OUTPUT} has $(wc -l ${BINDATA_OUTPUT}) lines of lovely automated artifacts"
else
	echo "No changes in generated bindata file: ${BINDATA_OUTPUT}"
fi

rm -f "${BINDATA_OUTPUT}.tmp"

popd >/dev/null
