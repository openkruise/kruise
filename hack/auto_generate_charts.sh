#!/bin/bash
set -o errexit
set -o nounset

# receive version version
echo "---------------------------------"
echo "Please input Kruise version (available versions are listed in https://github.com/openkruise/kruise/releases)"
echo "If no input, the default version of v0.3.0 will be used"
echo "---------------------------------"
read -p "please input version: " version

if [ ! -n "$version" ]; then
  version="v0.3.0"
fi

echo "----------------------------------"
echo "generate kurise helm charts version" $version

# generate chart folder
mkdir -p charts/templates

# download chart file
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/values.yaml -O charts/values.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/README.md -O charts/README.md
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/Chart.yaml -O charts/Chart.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/.helmignore -O charts/.helmignore

wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/_helpers.tpl -O charts/templates/_helpers.tpl
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/apps_v1alpha1_broadcastjob.yaml -O charts/templates/apps_v1alpha1_broadcastjob.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/apps_v1alpha1_sidecarset.yaml -O charts/templates/apps_v1alpha1_sidecarset.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/apps_v1alpha1_statefulset.yaml -O charts/templates/apps_v1alpha1_statefulset.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/manager.yaml -O charts/templates/manager.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/rbac_role.yaml -O charts/templates/rbac_role.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/rbac_role_binding.yaml -O charts/templates/rbac_role_binding.yaml
wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/webhookconfiguration.yaml -O charts/templates/webhookconfiguration.yaml

if [ "v0.3.0" == "$version" ]; then
  wget https://raw.githubusercontent.com/openkruise/kruise/master/charts/kruise/$version/templates/apps_v1alpha1_uniteddeployment.yaml -O charts/templates/apps_v1alpha1_uniteddeployment.yaml
fi