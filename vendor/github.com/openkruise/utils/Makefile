# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: test

##@ Development

go_check:
	@scripts/check_go_version "1.18.0"

fmt: go_check ## Run go fmt against code.
	go fmt $(shell go list ./... | grep -v /vendor/)

vet: ## Run go vet against code.
	go vet $(shell go list ./... | grep -v /vendor/)

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: fmt vet ## Run tests
	mkdir -p ${ENVTEST_ASSETS_DIR}
	source ./scripts/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

