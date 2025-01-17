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

set -e
# default value
FOCUS_DEFAULT='\[apps\] StatefulSet'
FOCUS=${FOCUS_DEFAULT}
SKIP=""
TIMEOUT="60m"
PRINT_INFO=false
PARALLEL=true

while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        --focus)
            FOCUS="$2"
            shift # past argument
            shift # past value
            ;;
        --skip)
            if [ -z "$SKIP" ]; then
                SKIP="$2"
            else
                SKIP="$SKIP|$2"
            fi
            shift # past argument
            shift # past value
            ;;
        --timeout)
            TIMEOUT="$2"
            shift # past argument
            shift # past value
            ;;
        --print-info)
            PRINT_INFO=true
            shift # past argument
            ;;
        --disable-parallel)
            PARALLEL=false
            shift # past argument
            ;;
        *)
            echo "Unknown parameter passed: $1"
            exit 1
            ;;
    esac
done

config=${KUBECONFIG:-${HOME}/.kube/config}
export KUBECONFIG=${config}
make ginkgo
echo "installing tfjobs crds"
kubectl apply -f https://raw.githubusercontent.com/kubeflow/training-operator/refs/heads/v1.8-branch/manifests/base/crds/kubeflow.org_tfjobs.yaml

set +e
set -x
GINKGO_CMD="./bin/ginkgo -timeout $TIMEOUT -v"
if [ "$PARALLEL" = true ]; then
    GINKGO_CMD+=" -p"
fi
if [ -n "$FOCUS" ]; then
    GINKGO_CMD+=" --focus='$FOCUS'"
fi
if [ -n "$SKIP" ]; then
    GINKGO_CMD+=" --skip='$SKIP'"
fi
GINKGO_CMD+=" test/e2e"

bash -c "$GINKGO_CMD"
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

kubectl get pods -n kruise-system -l control-plane=daemon -o=jsonpath="{range .items[*]}{.metadata.namespace}{\"\t\"}{.metadata.name}{\"\n\"}{end}" | while read ns name;
do
  restartCount=$(kubectl get pod -n ${ns} ${name} --no-headers | awk '{print $4}')
  if [ "${restartCount}" -eq "0" ];then
      echo "Kruise-daemon has not restarted"
  else
      kubectl get pods -n ${ns} -l control-plane=daemon --no-headers
      echo "Kruise-daemon has restarted, abort!!!"
      kubectl logs -p -n ${ns} ${name}
      exit 1
  fi
done

if [ "$PRINT_INFO" = true ]; then
    if [ "$retVal" -ne 0 ];then
        echo "test fail, dump kruise-manager logs"
        while read pod; do
             kubectl logs -n kruise-system $pod
        done < <(kubectl get pods -n kruise-system -l control-plane=controller-manager --no-headers | awk '{print $1}')
        echo "test fail, dump kruise-daemon logs"
        while read pod; do
             kubectl logs -n kruise-system $pod
        done < <(kubectl get pods -n kruise-system -l control-plane=daemon --no-headers | awk '{print $1}')
    fi
fi

exit $retVal
