#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

go mod vendor
retVal=$?
if [ $retVal -ne 0 ]; then
    exit $retVal
fi

TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/openkruise/kruise/pkg/client
cp -r ./{apis,hack,vendor,go.mod,.git} "${TMP_DIR}"/src/github.com/openkruise/kruise/

chmod +x "${TMP_DIR}"/src/github.com/openkruise/kruise/vendor/k8s.io/code-generator/generate-internal-groups.sh
echo "tmp_dir: ${TMP_DIR}"

SCRIPT_ROOT="${TMP_DIR}"/src/github.com/openkruise/kruise
CODEGEN_PKG=${CODEGEN_PKG:-"${SCRIPT_ROOT}/vendor/k8s.io/code-generator"}

echo "source ${CODEGEN_PKG}/kube_codegen.sh"
source "${CODEGEN_PKG}/kube_codegen.sh"

echo "gen_helpers"
GOPATH=${TMP_DIR} GO111MODULE=off kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/apis"

echo "gen_client"
GOPATH=${TMP_DIR} GO111MODULE=off kube::codegen::gen_client \
    --with-watch \
    --output-dir "${SCRIPT_ROOT}/pkg/client" \
    --output-pkg "github.com/openkruise/kruise/pkg/client" \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/apis"

rm -rf ./pkg/client/{clientset,informers,listers}
mv "${TMP_DIR}"/src/github.com/openkruise/kruise/pkg/client/* ./pkg/client
rm -rf vendor