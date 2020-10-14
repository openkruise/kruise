#!/usr/bin/env bash

set -ex

if [ ! -d /usr/local/kubebuilder ]; then
  curl -sL https://go.kubebuilder.io/dl/2.3.1/"$(go env GOOS)/$(go env GOARCH)" | tar -xz -C /tmp/
  sudo mv /tmp/kubebuilder_2.3.1_"$(go env GOOS)_$(go env GOARCH)" /usr/local/kubebuilder
  export PATH=$PATH:/usr/local/kubebuilder/bin
fi

make manager test

[[ -z $(git status -s) ]] || (printf 'Existing modified/untracked files.\nPlease run "make generate manifests" and push again.\n'; exit 1)
