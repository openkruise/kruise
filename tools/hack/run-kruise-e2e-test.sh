#!/usr/bin/env bash
# Copyright (c) 2023 Alibaba Group Holding Ltd.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#      http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -ex

export KUBECONFIG=${HOME}/.kube/config
make ginkgo
set +e
#./bin/ginkgo -p -timeout 60m -v --focus='\[apps\] InplaceVPA' test/e2e
#./bin/ginkgo -p -timeout 60m -v --focus='\[apps\] CloneSet' test/e2e
./bin/ginkgo -p -timeout 60m -v --focus='\[apps\] (CloneSet|InplaceVPA)' test/e2e

retVal=$?
restartCount=$(kubectl get pod -n kruise-system -l control-plane=controller-manager --no-headers | awk '{print $4}')
if [ "${restartCount}" -eq "0" ];then
    echo "Kruise-manager has not restarted"
else
    kubectl get pod -n kruise-system -l control-plane=controller-manager --no-headers
    echo "Kruise-manager has restarted, abort!!!"
    kubectl get pod -n kruise-system --no-headers -l control-plane=controller-manager | awk '{print $1}' | xargs kubectl logs -p -n kruise-system
    exit 1
fi
exit $retVal