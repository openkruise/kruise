# Image URL to use all building/pushing image targets
IMG ?= openkruise/kruise-manager:test
# Platforms to build the image for
PLATFORMS ?= linux/amd64,linux/arm64,linux/arm
# Produce CRDs that work for API servers that supports v1beta1 CRD and conversion, requires k8s 1.13 or later.
CRD_OPTIONS ?= "crd"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

go_check:
	@scripts/check_go_version "1.13.0"

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go --enable-leader-election=false

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -
	echo "resources:\n- manager.yaml" > config/manager/kustomization.yaml

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt: go_check
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	@scripts/generate_client.sh
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./apis/..."

# Build the docker image
docker-build: test
	docker build --pull --no-cache . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# Build and push the multiarchitecture docker images and manifest.
docker-multiarch:
	docker buildx build --pull --no-cache --platform=$(PLATFORMS) --push . -t $(IMG)

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen-kruise-2))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	echo "replace sigs.k8s.io/controller-tools => github.com/openkruise/controller-tools v0.2.9-kruise.2" >> go.mod ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.9 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	mv $(GOBIN)/controller-gen $(GOBIN)/controller-gen-kruise-2 ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen-kruise-2
else
CONTROLLER_GEN=$(shell which controller-gen-kruise-2)
endif
