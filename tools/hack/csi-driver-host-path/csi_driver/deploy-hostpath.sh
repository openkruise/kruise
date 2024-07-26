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

# This script captures the steps required to successfully
# deploy the hostpath plugin driver.  This should be considered
# authoritative and all updates for this process should be
# done here and referenced elsewhere.

# The script assumes that kubectl is available on the OS path
# where it is executed.

set -e
set -o pipefail

BASE_DIR="$( cd "$( dirname "$0" )" && pwd )"

TEMP_DIR="$( mktemp -d )"
trap 'rm -rf ${TEMP_DIR}' EXIT

# KUBELET_DATA_DIR can be set to replace the default /var/lib/kubelet.
# All nodes must use the same directory.
default_kubelet_data_dir=/var/lib/kubelet
: ${KUBELET_DATA_DIR:=${default_kubelet_data_dir}}

# If set, the following env variables override image registry and/or tag for each of the images.
# They are named after the image name, with hyphen replaced by underscore and in upper case.
#
# - CSI_ATTACHER_REGISTRY
# - CSI_ATTACHER_TAG
# - CSI_NODE_DRIVER_REGISTRAR_REGISTRY
# - CSI_NODE_DRIVER_REGISTRAR_TAG
# - CSI_PROVISIONER_REGISTRY
# - CSI_PROVISIONER_TAG
# - CSI_SNAPSHOTTER_REGISTRY
# - CSI_SNAPSHOTTER_TAG
# - HOSTPATHPLUGIN_REGISTRY
# - HOSTPATHPLUGIN_TAG
#
# Alternatively, it is possible to override all registries or tags with:
# - IMAGE_REGISTRY
# - IMAGE_TAG
# These are used as fallback when the more specific variables are unset or empty.
#
# IMAGE_TAG=canary is ignored for images that are blacklisted in the
# deployment's optional canary-blacklist.txt file. This is meant for
# images which have known API breakages and thus cannot work in those
# deployments anymore. That text file must have the name of the blacklisted
# image on a line by itself, other lines are ignored. Example:
#
#     # The following canary images are known to be incompatible with this
#     # deployment:
#     csi-snapshotter
#
# Beware that the .yaml files do not have "imagePullPolicy: Always". That means that
# also the "canary" images will only be pulled once. This is good for testing
# (starting a pod multiple times will always run with the same canary image), but
# implies that refreshing that image has to be done manually.
#
# As a special case, 'none' as registry removes the registry name.
# Set VOLUME_MODE_CONVERSION_TESTS to "true" to enable the feature in external-provisioner.

# The default is to use the RBAC rules that match the image that is
# being used, also in the case that the image gets overridden. This
# way if there are breaking changes in the RBAC rules, the deployment
# will continue to work.
#
# However, such breaking changes should be rare and only occur when updating
# to a new major version of a sidecar. Nonetheless, to allow testing the scenario
# where the image gets overridden but not the RBAC rules, updating the RBAC
# rules can be disabled.
: ${UPDATE_RBAC_RULES:=true}
function rbac_version () {
    yaml="$1"
    image="$2"
    update_rbac="$3"

    if ! [ -f "$yaml" ]; then
        # Fall back to csi-hostpath-plugin.yaml for those deployments which do not
        # have individual pods for the sidecars.
        yaml="$(dirname "$yaml")/csi-hostpath-plugin.yaml"
    fi

    # get version from `image: quay.io/k8scsi/csi-attacher:v1.0.1`, ignoring comments
    version="$(sed -e 's/ *#.*$//' "$yaml" | grep "image:.*$image" | sed -e 's/ *#.*//' -e 's/.*://')"

    if $update_rbac; then
        # apply overrides
        varname=$(echo $image | tr - _ | tr a-z A-Z)
        eval version=\${${varname}_TAG:-\${IMAGE_TAG:-\$version}}
    fi

    # When using canary images, we have to assume that the
    # canary images were built from the corresponding branch.
    case "$version" in canary) version=master;;
                        *-canary) version="$(echo "$version" | sed -e 's/\(.*\)-canary/release-\1/')";;
    esac

    echo "$version"
}

# version_gt returns true if arg1 is greater than arg2.
# 
# This function expects versions to be one of the following formats:
#   X.Y.Z, release-X.Y.Z, vX.Y.Z
#
#   where X,Y, and Z are any number.
#
# Partial versions (1.2, release-1.2) work as well.
# The follow substrings are stripped before version comparison:
#   - "v"
#   - "release-"
#
# Usage:
# version_gt release-1.3 v1.2.0  (returns true)
# version_gt v1.1.1 v1.2.0  (returns false)
# version_gt 1.1.1 v1.2.0  (returns false)
# version_gt 1.3.1 v1.2.0  (returns true)
# version_gt 1.1.1 release-1.2.0  (returns false)
# version_gt 1.2.0 1.2.2  (returns false)
function version_gt() { 
    versions=$(for ver in "$@"; do ver=${ver#release-}; ver=${ver#kubernetes-}; echo ${ver#v}; done)
    greaterVersion=${1#"release-"};  
    greaterVersion=${greaterVersion#"kubernetes-"};
    greaterVersion=${greaterVersion#"v"}; 
    test "$(printf '%s' "$versions" | sort -V | head -n 1)" != "$greaterVersion"
}

function volume_mode_conversion () {
    [ "${VOLUME_MODE_CONVERSION_TESTS}" == "true" ]
}

# In addition, the RBAC rules can be overridden separately.
# For snapshotter 2.0+, the directory has changed.
SNAPSHOTTER_RBAC_RELATIVE_PATH="rbac.yaml"
if version_gt $(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-snapshotter.yaml" csi-snapshotter "${UPDATE_RBAC_RULES}") "v1.255.255"; then
	SNAPSHOTTER_RBAC_RELATIVE_PATH="csi-snapshotter/rbac-csi-snapshotter.yaml"
fi

CSI_PROVISIONER_RBAC_YAML="https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-provisioner.yaml" csi-provisioner false)/deploy/kubernetes/rbac.yaml"
: ${CSI_PROVISIONER_RBAC:=https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-provisioner.yaml" csi-provisioner "${UPDATE_RBAC_RULES}")/deploy/kubernetes/rbac.yaml}
CSI_ATTACHER_RBAC_YAML="https://raw.githubusercontent.com/kubernetes-csi/external-attacher/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-attacher.yaml" csi-attacher false)/deploy/kubernetes/rbac.yaml"
: ${CSI_ATTACHER_RBAC:=https://raw.githubusercontent.com/kubernetes-csi/external-attacher/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-attacher.yaml" csi-attacher "${UPDATE_RBAC_RULES}")/deploy/kubernetes/rbac.yaml}
CSI_SNAPSHOTTER_RBAC_YAML="https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-snapshotter.yaml" csi-snapshotter false)/deploy/kubernetes/${SNAPSHOTTER_RBAC_RELATIVE_PATH}"
: ${CSI_SNAPSHOTTER_RBAC:=https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-snapshotter.yaml" csi-snapshotter "${UPDATE_RBAC_RULES}")/deploy/kubernetes/${SNAPSHOTTER_RBAC_RELATIVE_PATH}}
CSI_RESIZER_RBAC_YAML="https://raw.githubusercontent.com/kubernetes-csi/external-resizer/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-resizer.yaml" csi-resizer false)/deploy/kubernetes/rbac.yaml"
: ${CSI_RESIZER_RBAC:=https://raw.githubusercontent.com/kubernetes-csi/external-resizer/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-resizer.yaml" csi-resizer "${UPDATE_RBAC_RULES}")/deploy/kubernetes/rbac.yaml}

CSI_EXTERNALHEALTH_MONITOR_RBAC_YAML="https://raw.githubusercontent.com/kubernetes-csi/external-health-monitor/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-plugin.yaml" csi-external-health-monitor-controller false)/deploy/kubernetes/external-health-monitor-controller/rbac.yaml"
: ${CSI_EXTERNALHEALTH_MONITOR_RBAC:=https://raw.githubusercontent.com/kubernetes-csi/external-health-monitor/$(rbac_version "${BASE_DIR}/hostpath/csi-hostpath-plugin.yaml" csi-external-health-monitor-controller "${UPDATE_RBAC_RULES}")/deploy/kubernetes/external-health-monitor-controller/rbac.yaml}

INSTALL_CRD=${INSTALL_CRD:-"false"}

# Some images are not affected by *_REGISTRY/*_TAG and IMAGE_* variables.
# The default is to update unless explicitly excluded.
update_image () {
    case "$1" in socat) return 1;; esac
}

run () {
    echo "$@" >&2
    "$@"
}

# rbac rules
echo "applying RBAC rules"
for component in CSI_PROVISIONER CSI_ATTACHER CSI_SNAPSHOTTER CSI_RESIZER CSI_EXTERNALHEALTH_MONITOR; do
    eval current="\${${component}_RBAC}"
    eval original="\${${component}_RBAC_YAML}"
    if [ "$current" != "$original" ]; then
        echo "Using non-default RBAC rules for $component. Changes from $original to $current are:"
        diff -c <(wget --quiet -O - "$original") <(if [[ "$current" =~ ^http ]]; then wget --quiet -O - "$current"; else cat "$current"; fi) || true
    fi

    # using kustomize kubectl plugin to add labels to he rbac files.
    # since we are deploying rbas directly with the url, the kustomize plugin only works with the local files
    # we need to add the files locally in temp folder and using kustomize adding labels it will be applied
    if [[ "${current}" =~ ^http:// ]] || [[ "${current}" =~ ^https:// ]]; then
      run curl "${current}" --output "${TEMP_DIR}"/rbac.yaml --silent --location
    else
        # Even for local files we need to copy because kustomize only supports files inside
        # the root of a kustomization.
        cp "${current}" "${TEMP_DIR}"/rbac.yaml
    fi

    cat <<- EOF > "${TEMP_DIR}"/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

commonLabels:
  app.kubernetes.io/instance: hostpath.csi.k8s.io
  app.kubernetes.io/part-of: csi-driver-host-path

resources:
- ./rbac.yaml
EOF

    run kubectl apply --kustomize "${TEMP_DIR}"
done

# deploy hostpath plugin and registrar sidecar
echo "deploying hostpath components"
for i in $(ls ${BASE_DIR}/hostpath/*.yaml | sort); do
    echo "   $i"
    if volume_mode_conversion; then
      sed -i -e 's/# end csi-provisioner args/- \"--prevent-volume-mode-conversion=true\"\n            # end csi-provisioner args/' $i
    fi
    modified="$(cat "$i" | sed -e "s;${default_kubelet_data_dir}/;${KUBELET_DATA_DIR}/;" | while IFS= read -r line; do
        nocomments="$(echo "$line" | sed -e 's/ *#.*$//')"
        if echo "$nocomments" | grep -q '^[[:space:]]*image:[[:space:]]*'; then
            # Split 'image: quay.io/k8scsi/csi-attacher:v1.0.1'
            # into image (quay.io/k8scsi/csi-attacher:v1.0.1),
            # registry (quay.io/k8scsi),
            # name (csi-attacher),
            # tag (v1.0.1).
            image=$(echo "$nocomments" | sed -e 's;.*image:[[:space:]]*;;')
            registry=$(echo "$image" | sed -e 's;\(.*\)/.*;\1;')
            name=$(echo "$image" | sed -e 's;.*/\([^:]*\).*;\1;')
            tag=$(echo "$image" | sed -e 's;.*:;;')

            # Variables are with underscores and upper case.
            varname=$(echo $name | tr - _ | tr a-z A-Z)

            # Now replace registry and/or tag, if set as env variables.
            # If not set, the replacement is the same as the original value.
            # Only do this for the images which are meant to be configurable.
            if update_image "$name"; then
                prefix=$(eval echo \${${varname}_REGISTRY:-${IMAGE_REGISTRY:-${registry}}}/ | sed -e 's;none/;;')
                if [ "$IMAGE_TAG" = "canary" ] &&
                   [ -f ${BASE_DIR}/canary-blacklist.txt ] &&
                   grep -q "^$name\$" ${BASE_DIR}/canary-blacklist.txt; then
                    # Ignore IMAGE_TAG=canary for this particular image because its
                    # canary image is blacklisted in the deployment blacklist.
                    suffix=$(eval echo :\${${varname}_TAG:-${tag}})
                else
                    suffix=$(eval echo :\${${varname}_TAG:-${IMAGE_TAG:-${tag}}})
                fi
                line="$(echo "$nocomments" | sed -e "s;$image;${prefix}${name}${suffix};")"
            fi
            echo "        using $line" >&2
        fi
        echo "$line"
    done)"
    if ! echo "$modified" | kubectl apply -f -; then
        echo "modified version of $i:"
        echo "$modified"
        exit 1
    fi
done

check_statefulset () (
    ready=$(kubectl get "statefulset/$1" -o jsonpath="{.status.readyReplicas}")
    if [ "$ready" ] && [ "$ready" -gt 0 ]; then
        return 0
    fi
    return 1
)

check_statefulsets () (
    while [ "$#" -gt 0 ]; do
        if ! check_statefulset "$1"; then
            return 1
        fi
        shift
    done
    return 0
)

# Wait until all StatefulSets of the deployment are ready.
# The assumption is that we use one or more of those.
statefulsets="$(kubectl get statefulsets -l app.kubernetes.io/instance=hostpath.csi.k8s.io -o jsonpath='{range .items[*]}{" "}{.metadata.name}{end}')"
cnt=0
while ! check_statefulsets $statefulsets; do
    if [ $cnt -gt 30 ]; then
        echo "Deployment:"
        (set +e; set -x; kubectl describe all,role,clusterrole,rolebinding,clusterrolebinding,serviceaccount,storageclass,csidriver --all-namespaces -l app.kubernetes.io/instance=hostpath.csi.k8s.io)
        echo
        echo "Pod logs:"
        kubectl get pods -l app.kubernetes.io/instance=hostpath.csi.k8s.io --all-namespaces -o=jsonpath='{range .items[*]}{.metadata.name}{" "}{range .spec.containers[*]}{.name}{" "}{end}{"\n"}{end}' | while read -r pod containers; do
            for c in $containers; do
                echo
                (set +e; set -x; kubectl logs $pod $c)
            done
        done
        echo
        echo "ERROR: hostpath deployment not ready after over 5min"
        exit 1
    fi
    echo $(date +%H:%M:%S) "waiting for hostpath deployment to complete, attempt #$cnt"
    cnt=$(($cnt + 1))
    sleep 10
done

# Create a test driver configuration in the place where the prow job
# expects it?
if [ "${CSI_PROW_TEST_DRIVER}" ]; then
    cp "${BASE_DIR}/test-driver.yaml" "${CSI_PROW_TEST_DRIVER}"

    # When testing late binding, pods must be forced to run on the
    # same node as the hostpath driver. external-provisioner currently
    # doesn't handle the case when the "wrong" node is chosen and gets
    # stuck permanently with:
    # error generating accessibility requirements: no topology key found on CSINode csi-prow-worker2
    node="$(kubectl get pods/csi-hostpathplugin-0 -o jsonpath='{.spec.nodeName}')"
    echo >>"${CSI_PROW_TEST_DRIVER}" "ClientNodeName: $node"
fi
