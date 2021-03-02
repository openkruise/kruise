#!/usr/bin/env bash

go mod vendor
retVal=$?
if [ $retVal -ne 0 ]; then
    exit $retVal
fi

set -e
TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/openkruise/kruise
cp -r ./{apis,hack,vendor} "${TMP_DIR}"/src/github.com/openkruise/kruise/

(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
    GOPATH=${TMP_DIR} GO111MODULE=off go run vendor/k8s.io/kube-openapi/cmd/openapi-gen/openapi-gen.go \
    -O openapi_generated -i ./apis/apps/pub -p github.com/openkruise/kruise/apis/apps/pub -h ./hack/boilerplate.go.txt \
    --report-filename ./violation_exceptions.list)
(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
    GOPATH=${TMP_DIR} GO111MODULE=off go run vendor/k8s.io/kube-openapi/cmd/openapi-gen/openapi-gen.go \
    -O openapi_generated -i ./apis/apps/v1alpha1 -p github.com/openkruise/kruise/apis/apps/v1alpha1 -h ./hack/boilerplate.go.txt \
    --report-filename ./violation_exceptions.list)
(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
    GOPATH=${TMP_DIR} GO111MODULE=off go run vendor/k8s.io/kube-openapi/cmd/openapi-gen/openapi-gen.go \
    -O openapi_generated -i ./apis/apps/v1beta1 -p github.com/openkruise/kruise/apis/apps/v1beta1 -h ./hack/boilerplate.go.txt \
    --report-filename ./violation_exceptions.list)

cp -f "${TMP_DIR}"/src/github.com/openkruise/kruise/apis/apps/pub/openapi_generated.go ./apis/apps/pub
cp -f "${TMP_DIR}"/src/github.com/openkruise/kruise/apis/apps/v1alpha1/openapi_generated.go ./apis/apps/v1alpha1
cp -f "${TMP_DIR}"/src/github.com/openkruise/kruise/apis/apps/v1beta1/openapi_generated.go ./apis/apps/v1beta1
