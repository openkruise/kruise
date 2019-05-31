#!/usr/bin/env bash

# All the kubebuilder commands executed in this project, must be recorded here.

kubebuilder init --domain kruise.io --license apache2 --owner "The Kruise Authors"

# statefulset.apps.kruise.io
kubebuilder create api --group apps --version v1alpha1 --kind StatefulSet --controller=true --resource=true --make=false
kubebuilder alpha webhook --group apps --version v1alpha1 --kind StatefulSet --type=mutating --operations=create,update
kubebuilder alpha webhook --group apps --version v1alpha1 --kind StatefulSet --type=validating --operations=create,update


