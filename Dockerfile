# Build the manager and daemon binaries
ARG BASE_IMAGE=alpine
ARG BASE_IMAGE_VERSION=3.17
FROM golang:1.18-alpine3.17 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY main.go main.go
COPY apis/ apis/
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GO111MODULE=on go build -mod=vendor -a -o manager main.go \
  && CGO_ENABLED=0 GO111MODULE=on go build -mod=vendor -a -o daemon ./cmd/daemon/main.go

ARG BASE_IMAGE
ARG BASE_IMAGE_VERSION
FROM ${BASE_IMAGE}:${BASE_IMAGE_VERSION}

RUN apk add --no-cache ca-certificates bash expat \
  && rm -rf /var/cache/apk/*

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/daemon ./kruise-daemon
ENTRYPOINT ["/manager"]
