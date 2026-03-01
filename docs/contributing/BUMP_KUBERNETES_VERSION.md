# Bump Kubernetes version

## Preparing: Compatibility matrix

| kubebuilder | controller-runtime | Kubernetes  |
|-------------|--------------------|-------------|
| 1.x         | 0.1.x              | 1.13        |
| 2.x         | 0.2.x ~ 0.6.x      | 1.14 ~ 1.18 |
| 3.x         | 0.7.x ~ ?          | 1.19 ~ ?    |

Which means the correspondence between controller-runtime and Kubernetes:

- controller-runtime v0.7  - Kubernetes v1.19
- controller-runtime v0.8  - Kubernetes v1.20
- controller-runtime v0.9  - Kubernetes v1.21
- controller-runtime v0.10 - Kubernetes v1.22
- ...

So, you should firstly make sure your local kubebuilder is 3.x,
and make clear which versions of Kubernetes and controller-runtime you are going to update.

Usually we bump them every two versions with the even minor version, such as Kubernetes 1.20, 1.22.

## Actions

### Step 1: update dependencies in go.mod

- Bump `k8s.io/kubernetes` to `v1.x.y`.
- Bump `k8s.io/xxx` in require and replace to `v0.x.y`, except `k8s.io/kube-openapi` and `k8s.io/utils`, you can ignore them for now.
- Bump `sigs.k8s.io/controller-runtime` to matched version with Kubernetes.

Tidy them and download:

```shell
$ go mod tidy

$ go mod vendor
```

### Step 2: solve incompatible problems

Build and test:

```shell
$ make build test
```

Solve all incompatible problems in the project.

### Step 3: fetch upstream changes of StatefulSet and DaemonSet

We hope the Advanced StatefulSet and Advanced DaemonSet in Kruise should be compatible with the upstream ones.
So you should look for the upstream changes and fetch them into Kruise.

This can be done in next individual PRs to make this bump-version-PR more clear and readable.
