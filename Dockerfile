# Build the manager and daemon binaries
ARG BASE_IMAGE=alpine
ARG BASE_IMAGE_VERSION=3.17
FROM golang:1.19-alpine3.17 as builder

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

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/daemon ./kruise-daemon

RUN set -eux; \
    mkdir -p /log /tmp && \
    chown -R nobody:nobody /log && \
    chown -R nobody:nobody /tmp && \
    chown -R nobody:nobody /manager && \
    apk --no-cache --update upgrade && \
    apk --no-cache add ca-certificates && \
    apk --no-cache add tzdata && \
    rm -rf /var/cache/apk/* && \
    update-ca-certificates && \
    echo "only include root and nobody user" && \
    echo -e "root:x:0:0:root:/root:/bin/ash\nnobody:x:65534:65534:nobody:/:/sbin/nologin" | tee /etc/passwd && \
    echo -e "root:x:0:root\nnobody:x:65534:" | tee /etc/group && \
    rm -rf /usr/local/sbin/* && \
    rm -rf /usr/local/bin/* && \
    rm -rf /usr/sbin/* && \
    rm -rf /usr/bin/* && \
    rm -rf /sbin/* && \
    rm -rf /bin/*

ENTRYPOINT ["/manager"]
