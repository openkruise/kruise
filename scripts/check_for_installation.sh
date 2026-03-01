#!/usr/bin/env bash

set -e

mkdir -p /tmp/kruise
TIMESTAMP=$(date +%s)
CONF_FILE=/tmp/kruise/install-check-config-${TIMESTAMP}.yaml
CR_FILE=/tmp/kruise/install-check-cr-${TIMESTAMP}.yaml
LOG_FILE=/tmp/kruise/install-check-${TIMESTAMP}.log

cat > "${CONF_FILE}" <<- EOM
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: enablewebhooks.check.kruise.io
spec:
  group: check.kruise.io
  version: v1alpha1
  scope: Cluster
  names:
    plural: enablewebhooks
    singular: enablewebhook
    kind: EnableWebhook

---

apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: kruise-check-install-webhook-configuration
webhooks:
- clientConfig:
    caBundle: Cg==
    url: https://127.0.0.1:9876/validating-kruise-check-install
  failurePolicy: Fail
  name: validating-kruise-check-install.kruise.io
  rules:
  - apiGroups:
    - check.kruise.io
    apiVersions:
    - v1alpha1
    operations:
    - CREATE
    resources:
    - enablewebhooks
  sideEffects: None
EOM

cat > "${CR_FILE}" <<- EOM
apiVersion: check.kruise.io/v1alpha1
kind: EnableWebhook
metadata:
  name: enablewebhook-test
spec:
  foo: bar
EOM

kubectl apply -f "${CONF_FILE}" >> "${LOG_FILE}" 2>&1

set +e

kubectl apply -f "${CR_FILE}" >> "${LOG_FILE}" 2>&1
RETURN_CODE=$?

if [[ ${RETURN_CODE} -eq 0 ]]; then
    kubectl delete -f "${CR_FILE}" >> "${LOG_FILE}" 2>&1
fi
kubectl delete -f "${CONF_FILE}" >> "${LOG_FILE}" 2>&1
rm -f "${CR_FILE}" "${CONF_FILE}"

if [[ ${RETURN_CODE} -eq 0 ]]; then
    echo "Failed to check for installation because of webhook not working."
    echo "Please check the arguments of your kube-apiserver, make sure that MutatingAdmissionWebhook and ValidatingAdmissionWebhook have been open."
    exit 1
fi

echo "Successfully check for installation, you can install kruise now."
