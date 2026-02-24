#!/usr/bin/env bash

if [ -z "$IMG" ]; then
  echo "no found IMG env"
  exit 1
fi

set -e

make kustomize
KUSTOMIZE=$(pwd)/bin/kustomize
(cd config/manager && "${KUSTOMIZE}" edit set image controller="${IMG}")
# Use config/kind overlay to ensure imagePullPolicy: IfNotPresent for local images
"${KUSTOMIZE}" build config/kind > /tmp/kruise-kustomization.yaml
echo -e "# Adds namespace to all resources.\nnamespace: kruise-system\n\nresources:\n- manager.yaml" > config/manager/kustomization.yaml
kubectl apply -f /tmp/kruise-kustomization.yaml
