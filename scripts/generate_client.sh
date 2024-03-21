#!/usr/bin/env bash

go mod vendor
retVal=$?
if [ $retVal -ne 0 ]; then
    exit $retVal
fi

set -e
TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/openkruise/kruise/pkg/client
cp -r ./{apis,hack,vendor,go.mod} "${TMP_DIR}"/src/github.com/openkruise/kruise/

(cd "${TMP_DIR}"/src/github.com/openkruise/kruise; \
    GOPATH=${TMP_DIR} GO111MODULE=off /bin/bash vendor/k8s.io/code-generator/generate-groups.sh all \
    github.com/openkruise/kruise/pkg/client github.com/openkruise/kruise/apis "apps:v1alpha1 apps:v1beta1 policy:v1alpha1" -h ./hack/boilerplate.go.txt)

rm -rf ./pkg/client/{clientset,informers,listers}
mv "${TMP_DIR}"/src/github.com/openkruise/kruise/pkg/client/* ./pkg/client
