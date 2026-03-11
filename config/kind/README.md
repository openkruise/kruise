# Local Kind Development Overlay

This configuration overlay is specifically designed for local development with `kind`.

## Purpose

When developing locally with `kind`, we often load locally built images directly into the cluster (e.g., via `kind load docker-image`).
The default manifests use `imagePullPolicy: Always`, which forces the kubelet to try and pull the image from a registry, failing for local images.

This overlay patches the `Deployment/controller-manager` and `DaemonSet/daemon` to use `imagePullPolicy: IfNotPresent`, ensuring that the locally loaded images are used.

## Usage

This overlay is used by `scripts/deploy_kind.sh` during the local installation process.
