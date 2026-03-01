#!/usr/bin/env bash

# delete webhook configurations
kubectl delete mutatingwebhookconfiguration kruise-mutating-webhook-configuration
kubectl delete validatingwebhookconfiguration kruise-validating-webhook-configuration

# delete kruise-manager and rbac rules
kubectl delete ns kruise-system
kubectl delete clusterrolebinding kruise-manager-rolebinding kruise-daemon-rolebinding
kubectl delete clusterrole kruise-manager-role kruise-daemon-role

# delete CRDs
kubectl get crd -o name | grep "customresourcedefinition.apiextensions.k8s.io/[a-z.]*.kruise.io" | grep -v ctrlmesh.kruise.io | xargs kubectl patch -p '{"spec":{"conversion":null}}'
kubectl get crd -o name | grep "customresourcedefinition.apiextensions.k8s.io/[a-z.]*.kruise.io" | grep -v ctrlmesh.kruise.io | xargs kubectl delete
