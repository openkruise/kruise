#!/bin/bash

set -e

if ! command -v gci &> /dev/null; then
    export PATH="$(go env GOPATH)/bin:$PATH"

    # If still not found, install gci
    if ! command -v gci &> /dev/null; then
        echo "Installing gci..."
        go install github.com/daixiang0/gci@latest
        export PATH="$(go env GOPATH)/bin:$PATH"
    fi
fi

# Format all Go files using gci with proper import ordering
# NOTE: Exclude "./test/e2e/e2e_test.go" to preserve ginkgo test suite detection
# Do not use gci to format "./test/e2e/e2e_test.go" as it will remove ginkgo comment line that are necessary,
# otherwise ginkgo test suite detection will fail
# See https://github.com/kubernetes/kubernetes/issues/74827
find . -name "*.go" -not -path "./test/e2e/e2e_test.go" | xargs gci write --skip-generated \
    -s standard \
    -s default \
    -s "prefix(github.com/openkruise/kruise)"
