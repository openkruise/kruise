#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script performs the step to destroy the hostpath plugin driver
# completely by checking all the resources.
# This should be in sync with the deploy script based on any future additions/deletions
# of the resources

# The script assumes that kubectl is available on the OS path
# where it is executed.

set -e
set -o pipefail

# Deleting all the resources installed by the deploy-script as the part of csi-hostpath-driver.
# Every resource in the driver installation has the label representing the installation instance.
# Using app.kubernetes.io/instance: hostpath.csi.k8s.io and app.kubernetes.io/part-of: csi-driver-host-path labels to identify the installation set
#
# kubectl maintains a few standard resources under "all" category which can be deleted by using just "kubectl delete all"
# and other resources such as roles, clusterrole, serivceaccount etc needs to be deleted explicitly
kubectl delete all --all-namespaces -l app.kubernetes.io/instance=hostpath.csi.k8s.io,app.kubernetes.io/part-of=csi-driver-host-path --wait=true
kubectl delete role,clusterrole,rolebinding,clusterrolebinding,serviceaccount,storageclass,csidriver --all-namespaces -l app.kubernetes.io/instance=hostpath.csi.k8s.io,app.kubernetes.io/part-of=csi-driver-host-path --wait=true
