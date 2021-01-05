#!/usr/bin/env bash

if [ -z "$IMG" ]; then
  echo "no found IMG env"
  exit 1
fi

set -e

(cd config/manager && kustomize edit set image controller="${IMG}")
kustomize build config/default | sed -e 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' > /tmp/kruise-kustomization.yaml
echo -e "resources:\n- manager.yaml" > config/manager/kustomization.yaml
kubectl apply -f /tmp/kruise-kustomization.yaml
