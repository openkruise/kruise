
# Image URL to use all building/pushing image targets
IMG ?= openkruise/kruise-manager:test
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

CURRENT_DIR=$(shell pwd)

all: test manager

go_check:
	@scripts/check_go_version "1.12.0"

# Run tests
test: generate fmt vet manifests kubebuilder
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager github.com/openkruise/kruise/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role paths="./..." output:crd:artifacts:config=config/crds

# Run go fmt against code
fmt: go_check
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./pkg/apis/..."
	go run ./hack/gen-openapi-spec/main.go > ${CURRENT_DIR}/api/openapi-spec/swagger.json

# kubebuilder binary
kubebuilder:
ifeq (, $(shell which kubebuilder))
	# Download kubebuilder and extract it to tmp
	curl -sL https://go.kubebuilder.io/dl/1.0.8/$(shell go env GOOS)/$(shell go env GOARCH) | tar -xz -C /tmp/
	# You'll need to set the KUBEBUILDER_ASSETS env var if you put it somewhere else
	sudo mv /tmp/kubebuilder_1.0.8_$(shell go env GOOS)_$(shell go env GOARCH) /usr/local/kubebuilder
newPATH:=$(PATH):/usr/local/kubebuilder/bin
export PATH=$(newPATH)
endif

# Build the docker image
docker-build:
	go mod vendor
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen-kruise))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	echo "replace sigs.k8s.io/controller-tools => github.com/openkruise/controller-tools v0.2.9-kruise" >> go.mod ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.9 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	mv $(GOPATH)/bin/controller-gen $(GOPATH)/bin/controller-gen-kruise ;\
	}
CONTROLLER_GEN=$(GOPATH)/bin/controller-gen-kruise
else
CONTROLLER_GEN=$(shell which controller-gen-kruise)
endif
