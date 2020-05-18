# OpenKruise/Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)

[English](./README.md) | 简体中文

|![notification](docs/img/bell-outline-badge.svg) 最新进展：|
|------------------|
|Mar 20th, 2020. Kruise v0.4.1 发布! 为 Advanced StatefulSet 和 CloneSet 提供了 **优雅原地升级** 功能，详情参见 [CHANGELOG](CHANGELOG.md).|
|Feb 7th,  2020. Kruise v0.4.0 发布! **新增 CloneSet 控制器**，详情参见 [CHANGELOG](CHANGELOG.md).|
|Nov 24th, 2019. 发布 UnitedDeployment 控制器的博客 ([link](http://openkruise.io/en-us/blog/blog3.html)).|

Kruise 是 OpenKruise 中的核心项目之一，它提供一套在[Kubernetes核心控制器](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/)之外的扩展 workload 管理和实现。

目前，Kruise 提供了以下 5 个 workload 控制器：

- [CloneSet](./docs/concepts/cloneset/README.md): 提供了更加高效、确定可控的应用管理和部署能力，支持优雅原地升级、指定删除、发布顺序可配置、并行/灰度发布等丰富的策略，可以满足更多样化的应用场景。

- [Advanced StatefulSet](./docs/concepts/astatefulset/README.md): 基于原生 [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) 之上的增强版本，默认行为与原生完全一致，在此之外提供了原地升级、并行发布（最大不可用）、发布暂停等功能。

- [SidecarSet](./docs/concepts/sidecarSet/README.md): 对 sidecar 容器做统一管理，在满足 selector 条件的 Pod 中注入指定的 sidecar 容器。

- [UnitedDeployment](./docs/concepts/uniteddeployment/README.md): 通过多个 subset workload 将应用部署到多个可用区。

- [BroadcastJob](./docs/concepts/broadcastJob/README.md): 配置一个 job，在集群中所有满足条件的 Node 上都跑一个 Pod 任务。

项目的 **roadmap** 参考[这里](https://github.com/openkruise/kruise/projects)。
[Video](https://www.youtube.com/watch?v=elB7reZ6eAQ) by [Lachlan Evenson](https://github.com/lachie83) 是一个对于新人很友好的 demo。

## 开始使用

### 安装前检查

使用 Kruise 需要在 `kube-apiserver` 启用一些 feature-gate 比如 `MutatingAdmissionWebhook`、`ValidatingAdmissionWebhook` （K8s 1.12以上默认开启）。
如果你的 K8s 版本低于 1.12，需要先执行以下命令来验证是否支持：

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/openkruise/kruise/master/scripts/check_for_installation.sh)"
```

### 使用 helm charts 安装 [推荐]

推荐使用 helm v3 安装 Kruise，helm 是一个简单的命令行工具可以从[这里](https://github.com/helm/helm/releases) 获取。

```
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.4.1/kruise-chart.tgz
```

注意直接安装 chart 会使用默认的 template values，你也可以根据你的集群情况指定一些特殊配置，比如修改 resources 限制或者只启用某些特定的控制器能力。

### 使用 YAML files 安装 [不推荐]

```bash
# Install CRDs
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_uniteddeployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_cloneset.yaml

# Install kruise-controller-manager
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml
```

注意 `all_in_one.yaml`  中包含的 Kruise-manager 镜像是每天周期性从 master 分支打出来的，无法保证功能的稳定性。
所以你可以通过 YAML 部署到测试集群做验证，但不推荐在生产环境使用。

官方的 kruise-manager 镜像维护在 [docker hub](https://hub.docker.com/r/openkruise/kruise-manager) 。

### 可选: 启用部分特定控制器

如果你只需要使用某些 Kruise 中的控制器并关闭其他的控制器，你可以做以下两个方式或同时做：

1. 只安装你需要使用的 CRD。

2. 在 kruise-manager 容器中设置 `CUSTOM_RESOURCE_ENABLE` 环境变量，配置需要启用的功能，比如 `CUSTOM_RESOURCE_ENABLE=CloneSet,StatefulSet`。

如果使用 helm chart 安装，可以通过以下参数来生效这个配置：

```
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.4.1/kruise-chart.tgz --set manager.custom_resource_enable="CloneSet\,StatefulSet"
```

## 使用说明

详见 [documents](./docs/README.md) 包含了各个 workload 的说明和用例，
我们也提供了 [**tutorials**](./docs/tutorial/README.md) 来示范如何使用 Kruise 控制器。

## 卸载

注意：卸载会导致所有 Kruise 下的资源都会删除掉，包括 webhook configurations, services, namespace, CRDs, CR instances 以及所有 Kruise workload 下的 Pod。
请务必谨慎操作！

卸载使用 helm chart 安装的 Kruise：

```bash
helm uninstall kruise
```

卸载使用 YAML files 安装的 Kruise:

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/kruiseio/kruise/master/scripts/uninstall.sh)"
```

## 社区

如果有任何问题或想要参与贡献，我们非常欢迎你在 Github 上提出 issues 或是 pull requests。

其他活跃的社区途径：

- Slack: [channel address](https://join.slack.com/t/kruise-workspace/shared_invite/enQtNjU5NzQ0ODcyNjYzLWJlZGJiZjUwNGU5Y2U2ODI3N2JiODI4N2M1OWFlOTgzMDgyOWVkZGRjNzdmZTBjYzgxZmM5MjAyNjhhZTdmMjQ)
- 钉钉讨论群

<div align="center">
  <img src="docs/img/openkruise-dev-group.JPG" width="250" title="dingtalk">
</div>

## Copyright

Certain implementation relies on existing code from Kubernetes and the credit goes to original Kubernetes authors.
