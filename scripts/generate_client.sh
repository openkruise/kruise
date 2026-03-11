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
cp -r ./{apis,hack,vendor,go.mod,go.sum,.git} "${TMP_DIR}"/src/github.com/openkruise/kruise/

chmod +x "${TMP_DIR}"/src/github.com/openkruise/kruise/vendor/k8s.io/code-generator/generate-internal-groups.sh
echo "tmp_dir: ${TMP_DIR}"

SCRIPT_ROOT="${TMP_DIR}"/src/github.com/openkruise/kruise
CODEGEN_PKG=${CODEGEN_PKG:-"${SCRIPT_ROOT}/vendor/k8s.io/code-generator"}

# Install code-generator tools with module mode (required for k8s 1.35+)
echo "Installing code-generator tools..."
(
    cd "${SCRIPT_ROOT}"
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/deepcopy-gen@v0.35.0
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/defaulter-gen@v0.35.0
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/conversion-gen@v0.35.0
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/client-gen@v0.35.0
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/lister-gen@v0.35.0
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/informer-gen@v0.35.0
    GOFLAGS="" GO111MODULE=on go install k8s.io/code-generator/cmd/validation-gen@v0.35.0
)

echo "source ${CODEGEN_PKG}/kube_codegen.sh"
source "${CODEGEN_PKG}/kube_codegen.sh"

echo "gen_helpers"
# GOFLAGS="-mod=mod" forces module mode even when vendor directory exists
GOPATH=${TMP_DIR} GO111MODULE=on GOFLAGS="-mod=mod" kube::codegen::gen_helpers \
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