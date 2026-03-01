#!/usr/bin/env bash

go mod vendor
retVal=$?
if [ $retVal -ne 0 ]; then
    exit $retVal
fi

set -e
TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/openkruise/kruise
cp -r ./{apis,hack,vendor,go.mod,.git} "${TMP_DIR}"/src/github.com/openkruise/kruise/
echo "tmp_dir: ${TMP_DIR}"
SCRIPT_ROOT="${TMP_DIR}"/src/github.com/openkruise/kruise
CODEGEN_PKG=${CODEGEN_PKG:-"${SCRIPT_ROOT}/vendor/k8s.io/code-generator"}
source "${CODEGEN_PKG}/kube_codegen.sh"

echo "gen_openapi"
report_filename="./violation_exceptions.list"

(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
GOPATH=${TMP_DIR} kube::codegen::gen_openapi \
    --output-dir "${SCRIPT_ROOT}/apis/apps/v1alpha1" \
    --output-pkg "github.com/openkruise/kruise/apis/apps/v1alpha1" \
    --report-filename "${report_filename}" \
    --update-report \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/apis/apps/v1alpha1")

(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
GOPATH=${TMP_DIR} kube::codegen::gen_openapi \
    --output-dir "${SCRIPT_ROOT}/apis/apps/v1beta1" \
    --output-pkg "github.com/openkruise/kruise/apis/apps/v1beta1" \
    --report-filename "${report_filename}" \
    --update-report \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/apis/apps/v1beta1")

(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
GOPATH=${TMP_DIR} kube::codegen::gen_openapi \
    --output-dir "${SCRIPT_ROOT}/apis/apps/pub" \
    --output-pkg "github.com/openkruise/kruise/apis/apps/pub" \
    --report-filename "${report_filename}" \
    --update-report \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/apis/apps/pub")

cp -f "${TMP_DIR}"/src/github.com/openkruise/kruise/apis/apps/pub/zz_generated.openapi.go ./apis/apps/pub/openapi_generated.go
cp -f "${TMP_DIR}"/src/github.com/openkruise/kruise/apis/apps/v1alpha1/zz_generated.openapi.go ./apis/apps/v1alpha1/openapi_generated.go
cp -f "${TMP_DIR}"/src/github.com/openkruise/kruise/apis/apps/v1beta1/zz_generated.openapi.go ./apis/apps/v1beta1/openapi_generated.go
rm -rf vendor 
