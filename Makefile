# Image URL to use all building/pushing image targets
IMG ?= openkruise/kruise-manager:test
# Platforms to build the image for
PLATFORMS ?= linux/amd64,linux/arm64,linux/ppc64le
CRD_OPTIONS ?= "crd:crdVersions=v1"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
GOOS ?= $(shell go env GOOS)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
# Run `setup-envtest list` to list available versions.
ENVTEST_K8S_VERSION ?= 1.28.0

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ Development

go_check:
	@scripts/check_go_version "1.20"

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	@scripts/generate_client.sh
	@scripts/generate_openapi.sh
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./apis/..."

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

fmt: go_check ## Run go fmt against code.
	go fmt $(shell go list ./... | grep -v /vendor/)

vet: ## Run go vet against code.
	go vet $(shell go list ./... | grep -v /vendor/)

lint: golangci-lint ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run

test: generate fmt vet manifests envtest ## Run tests
	echo $(ENVTEST)
	go build -o pkg/daemon/criruntime/imageruntime/fake_plugin/fake-credential-plugin pkg/daemon/criruntime/imageruntime/fake_plugin/main.go && chmod +x pkg/daemon/criruntime/imageruntime/fake_plugin/fake-credential-plugin
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -race ./pkg/... -coverprofile cover.out
	rm pkg/daemon/criruntime/imageruntime/fake_plugin/fake-credential-plugin

coverage-report: ## Generate cover.html from cover.out
	go tool cover -html=cover.out -o cover.html
ifeq ($(GOOS), darwin)
	open ./cover.html
else
	echo "open cover.html with a HTML viewer."
endif

##@ Build

build: generate fmt vet manifests ## Build manager binary.
	go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

docker-build: ## Build docker image with the manager.
	docker build --pull --no-cache . -t ${IMG}

docker-push: ## Push docker image with the manager.
	docker push ${IMG}

# Build and push the multiarchitecture docker images and manifest.
docker-multiarch:
	docker buildx build -f ./Dockerfile_multiarch --pull --no-cache --platform=$(PLATFORMS) --push . -t $(IMG)

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -
	$(KUSTOMIZE) build config/daemonconfig | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -
	$(KUSTOMIZE) build config/daemonconfig | kubectl delete -f -

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.

# controller-gen@v0.14.0 comply with k8s.io/api v0.28.x
ifeq ("$(shell $(CONTROLLER_GEN) --version 2> /dev/null)", "Version: v0.14.0")
else
	rm -rf $(CONTROLLER_GEN)
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0)
endif
KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.51.2)

GINKGO = $(shell pwd)/bin/ginkgo
ginkgo: ## Download ginkgo locally if necessary.
	$(call go-get-tool,$(GINKGO),github.com/onsi/ginkgo/ginkgo@v1.16.4)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2) to $(PROJECT_DIR)/bin" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

include tools/tools.mk

## Location to install dependencies to
TESTBIN ?= $(shell pwd)/testbin
$(TESTBIN):
	mkdir -p $(TESTBIN)

ENVTEST ?= $(TESTBIN)/setup-envtest

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(TESTBIN)
ifeq (, $(shell ls $(TESTBIN)/setup-envtest 2>/dev/null))
	GOBIN=$(TESTBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@c7e1dc9b5302d649d5531e19168dd7ea0013736d
endif

# create-cluster creates a kube cluster with kind.
.PHONY: create-cluster
create-cluster: $(tools/kind)
	tools/hack/create-cluster.sh

DISABLE_CSI ?= false

.PHONY: install-csi
install-csi:
ifeq ($(DISABLE_CSI), true)
	@echo "CSI is disabled, skip"
else
	cd tools/hack/csi-driver-host-path; ./install-snapshot.sh
endif

# delete-cluster deletes a kube cluster.
.PHONY: delete-cluster
delete-cluster: $(tools/kind) ## Delete kind cluster.
	$(tools/kind) delete cluster --name ci-testing

# kube-load-image loads a local built docker image into kube cluster.
.PHONY: kube-load-image
kube-load-image: $(tools/kind)
	tools/hack/kind-load-image.sh $(IMG)

# install-kruise install kruise with local build image to kube cluster.
.PHONY: install-kruise
install-kruise:
	tools/hack/install-kruise.sh $(IMG)

# run-kruise-e2e-test starts to run kruise e2e tests.
.PHONY: run-kruise-e2e-test
run-kruise-e2e-test:
	@echo -e "\n\033[36mRunning kruise e2e tests...\033[0m"
	tools/hack/run-kruise-e2e-test.sh

generate_helm_crds:
	scripts/generate_helm_crds.sh

# kruise-e2e-test runs kruise e2e tests.
.PHONY: kruise-e2e-test
kruise-e2e-test: $(tools/kind) delete-cluster create-cluster install-csi docker-build kube-load-image install-kruise run-kruise-e2e-test delete-cluster
