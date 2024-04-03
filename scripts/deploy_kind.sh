#!/usr/bin/env bash

if [ -z "$IMG" ]; then
  echo "no found IMG env"
  exit 1
fi

set -e

make kustomize
KUSTOMIZE=$(pwd)/bin/kustomize
(cd config/manager && "${KUSTOMIZE}" edit set image controller="${IMG}")
"${KUSTOMIZE}" build config/default | sed -e 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' > /tmp/kruise-kustomization.yaml
echo -e "# Adds namespace to all resources.\nnamespace: kruise-system\n\nresources:\n- manager.yaml" > config/manager/kustomization.yaml
kubectl apply -f /tmp/kruise-kustomization.yaml
