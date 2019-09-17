
# Image URL to use all building/pushing image targets
IMG ?= openkruise/kruise-manager:test

CURRENT_DIR=$(shell pwd)

all: test manager

go_check:
	@scripts/check_go_version "1.11.1"

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
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt: go_check
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	go generate ./pkg/... ./cmd/...
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
docker-build: #test
	docker build . -t ${IMG}
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml ./config/manager/all_in_one.yaml

# Push the docker image
docker-push:
	docker push ${IMG}
