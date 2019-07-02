# OpenKruise/Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![codecov](https://codecov.io/gh/openkruise/kruise/branch/master/graph/badge.svg)](https://codecov.io/gh/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)

Kruise 是 OpenKruise 项目的核心。 它是一组控制器，可在 workload 管理上扩展和补充 [kubernetes 核心控制器](https://jimmysong.io/kubernetes-handbook/)。

现在，Kruise 提供三种 workload 控制器：

* [Advanced StatefulSet](./docs/concepts/astatefulset/README.md)： 默认 [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) 的增强版本，具有额外的功能，例如 `原地更新`。

* [BroadcastJob](./docs/concepts/broadcastJob/README.md): 在集群中的所有节点上运行 pod 以完成作业。

* [SidecarSet](./docs/concepts/sidecarSet/README.md): 根据选择器将 sidecar 容器注入到 pod spec。

有关更多技术信息，请参阅[文档](./docs/README.md)

这里提供了几个[教程](./docs/tutorial/README.md)来演示如何使用 workload 控制器。

## 入门

### 使用 YAML 文件安装

#### 安装 CRDs

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
```

请注意，需要安装全部的CRD，才能使 kruise-controller 正常运行。

##### 安装 kruise-controller-manager

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml`

或者从 repo 根目录运行：

`kustomize build config/default | kubectl apply -f -`

要安装 kustomize，可以从 kustomize [网站](https://github.com/kubernetes-sigs/kustomize)下载。

注意使用 Kustomize 1.0.11版本， 2.0.3版本与 kube-builder 存在兼容性问题。

官方的 kruise-controller-manager 镜像托管在 [docker Hub](https://hub.docker.com/r/openkruise/kruise-manager)。

## 使用示例

### Advanced StatefulSet

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: StatefulSet
metadata:
  name: sample
spec:
  replicas: 3
  serviceName: fake-service
  selector:
    matchLabels:
      app: sample
  template:
    metadata:
      labels:
        app: sample
    spec:
      readinessGates:
        # A new condition must be added to ensure the pod remain at NotReady state while the in-place update is happening
      - conditionType: InPlaceUpdateReady
      containers:
      - name: main
        image: nginx:alpine
  podManagementPolicy: Parallel  # allow parallel updates, works together with maxUnavailable
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      # Do in-place update if possible, currently only image update is supported for in-place update
      podUpdatePolicy: InPlaceIfPossible
      # Allow parallel updates with max number of unavailable instances equals to 2
      maxUnavailable: 2
```

### Broadcast Job

Run a BroadcastJob that each Pod computes pi, with `ttlSecondsAfterFinished` set to 30. The job
will be deleted in 30 seconds after the job is finished.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: BroadcastJob
metadata:
  name: broadcastjob-ttl
spec:
  template:
    spec:
      containers:
        - name: pi
          image: perl
          command: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
      restartPolicy: Never
  completionPolicy:
    type: Always
    ttlSecondsAfterFinished: 30
```

### SidecarSet

The yaml file below describes a SidecarSet that contains a sidecar container named `sidecar1`

```yaml
# sidecarset.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: test-sidecarset
spec:
  selector: # select the pods to be injected with sidecar containers
    matchLabels:
      app: nginx
  containers:
  - name: sidecar1
    image: centos:7
    command: ["sleep", "999d"] # do nothing at all
```

## 开发者指南

根文件夹中有一个 `makefile` 文件，描述了要生成和安装的选项。以下是常见的选项：

构建 controller manager 二进制文件

`make manager`

运行测试

`make test`

生成容器镜像， 默认镜像名是 `openkruise/kruise-manager:v1alpha1`

`export IMG=<your_image_name> && make docker-build`

上传镜像

`export IMG=<your_image_name> && make docker-push`
或者
`docker push <your_image_name>`

生成清单，如 CRD、RBAC 等

`make manifests`

## 社区

如果您有任何问题或想做出贡献，欢迎加入我们 [slack channel](https://join.slack.com/t/kruise-workspace/shared_invite/enQtNjU5NzQ0ODcyNjYzLWMzZDI5NTM3ZjM1MGY2Mjg1NzU4ZjBjMDJmNjZmZTEwYTZkMzk4ZTAzNmY5NTczODhkZDU2NzVhM2I2MzNmODc)。

邮件列表: todo

## 版权

某些实现依赖于 kubernetes 的现有代码，这归功于 kubernetes 作者。