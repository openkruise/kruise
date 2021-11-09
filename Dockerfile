# Build the manager binary
FROM golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
#RUN go mod download

# Copy the go source
COPY vendor/ vendor/
COPY apis/ apis/
COPY cmd/ cmd/
COPY pkg/ pkg/
# Build
RUN CGO_ENABLED=0 GO111MODULE=on GOBIN=/workspace/ go install -mod=vendor -a ./cmd/...

# Use Ubuntu 20.04 LTS as base image to package the manager binary
FROM ubuntu:focal
# This is required by daemon connnecting with cri
RUN ln -s /usr/bin/* /usr/sbin/ && apt-get update -y \
  && apt-get install --no-install-recommends -y ca-certificates \
  && apt-get clean && rm -rf /var/log/*log /var/lib/apt/lists/* /var/log/apt/* /var/lib/dpkg/*-old /var/cache/debconf/*-old
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/daemon ./kruise-daemon
ENTRYPOINT ["/manager"]
