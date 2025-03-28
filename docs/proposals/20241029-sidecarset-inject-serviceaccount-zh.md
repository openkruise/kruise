---
title: AdvancedStatefulSetVolumeResize
authors:
  - "@magicsong"
reviewers:
  - "@furykerry"
  - "@zmberg"
creation-date: 2024-10-29
last-updated: 2024-11-06
status: 

---
# SidecarSet 中的 ServiceAccount 自动注入
## 目录

- [SidecarSet 中的 ServiceAccount 自动注入](#sidecarset-中的-serviceaccount-自动注入)
  - [目录](#目录)
  - [摘要](#摘要)
  - [动机](#动机)
    - [目标](#目标)
    - [非目标](#非目标)
  - [提案](#提案)
    - [用户故事](#用户故事)
    - [注意事项/约束/警告](#注意事项约束警告)
    - [风险和缓解措施](#风险和缓解措施)
  - [设计细节](#设计细节)
    - [API 变更](#api-变更)
  - [实现策略](#实现策略)
    - [Validation阶段：](#validation阶段)
    - [Mutation阶段：](#mutation阶段)
    - [毕业标准](#毕业标准)
    - [缺点](#缺点)
    - [替代方案](#替代方案)

---

## 摘要

此 KEP 提议在 Kruise 的 SidecarSet 中注入 ServiceAccount 的功能。此功能将允许由 Kruise 的 SidecarSet 管理的 sidecar 容器使用特定的 ServiceAccounts，从而实现更好的访问控制和安全配置。

## 动机

目前，由 SidecarSet 注入的 sidecar 继承了主应用容器的 ServiceAccount。在某些使用场景中，sidecar 可能需要不同的权限，并且应该能够指定自己的 ServiceAccounts。此提案旨在为 SidecarSets 中的 ServiceAccounts 注入提供解决方案，使用户能够更细致地控制 sidecar 的权限。

相关问题：https://github.com/openkruise/kruise/issues/1747

### 目标

- 允许用户为由 SidecarSet 管理的 sidecar 容器指定 ServiceAccount。
- 提高 sidecar 权限管理的安全性和灵活性。

### 非目标

- 彻底改革 Kubernetes 中现有的 ServiceAccount 系统。
- 提供超出 ServiceAccount 级别的基于角色或权限的配置。

## 提案

### 用户故事

- **故事 1**：作为用户，我希望为由 SidecarSet 注入的日志 sidecar 分配一个专用的 ServiceAccount，以限制其对某些资源的访问，而不影响主应用容器。
- **故事 2**：作为开发人员，我希望 SidecarSet 根据每个 sidecar 所需的权限注入不同的 ServiceAccounts。一个 sidecar 容器需要访问 Kubernetes 资源（如 ConfigMaps、Secrets）或与 API 服务器交互，因此需要具有适当权限的 ServiceAccount。通过 SidecarSet 直接注入 ServiceAccount 简化了这一过程并使其更易于管理。

### 注意事项/约束/警告

此功能必须确保与现有的 Kruise ServiceAccount 管理兼容，并避免在更新 ServiceAccounts 时发生冲突。

### 风险和缓解措施

- **风险**：潜在的错误配置可能导致 sidecar 运行时具有意外的权限。
- **缓解措施**：添加验证以确保仅指定允许的 ServiceAccounts。

## 设计细节

### API 变更

1. 在 SidecarSet 规范中添加一个新的 `serviceAccountName` 字段，允许用户为 sidecar 容器定义一个特定的 ServiceAccount。
2. 在 SidecarSet 规范中添加一个新的 `behaviorWhenServiceAccountInjection` 字段，以控制当多个 SidecarSets 向同一个 pod 注入不同的 ServiceAccounts 时的行为。
   1. Refuse：拒绝注入，返回错误。
   2. Merge：合并权限注入，同时选择一个合适的ServiceAccountName
3. 在SidecarSet中添加一个新的 `serviceAccountPermissions` 字段，允许kruise 通过Merge的方式解决ServiceAccount问题

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: sidecarset-sample
spec:
    serviceAccountName: "sidecar-service-account"
    behaviorWhenServiceAccountInjection: "refuse"
    serviceAccountPermissions:
    - apiGroups: [""]
      resources: ["pods", "pods/log"]
      verbs: ["get", "list", "watch"]
    - apiGroups: ["apps"]
      resources: ["deployments"]
      verbs: ["get", "list", "watch"]
    - apiGroups: ["batch"]
      resources: ["jobs"]
      verbs: ["create", "delete"]
    containers:
    - name: sidecar
        image: sidecar-image
        ...
    - name: main-app
        image: main-app-image
        ...
```

## 实现策略
### Validation阶段：

1. 确保仅在启用相关 FeatureGate 时处理 serviceAccountName 字段。
2. 验证指定的 ServiceAccount 不是默认的 ServiceAccount。
3. 检查 ServiceAccount 是否存在。不存在的ServiceAccount没有意义
4. 检查如果多个 SidecarSets 尝试向同一个 pod 注入 ServiceAccounts 时是否存在冲突。

### Mutation阶段：

1. 修改 Kruise SidecarSet 控制器以解释新的 serviceAccountName 字段。
2. 确保应用容器未设置自己的 ServiceAccount，或设置为默认值。
3. 移除 Kubernetes 自动注入的 ServiceAccount 挂载（如果存在）。
4. 冲突解决(如果只有一个SIdecarSet或者没有则跳过)：
  + behaviorWhenServiceAccountInjection 实现逻辑以处理和解决当多个 SidecarSets 向同一个 pod 注入不同的 ServiceAccounts 时的行为，包括拒绝注入和合并权限注入。如果存在一个SidecarSet是拒绝，则整体就是拒绝
  + 合并权限需要Kruise创建对应的ServiceAccount和role，以及roleBinding，同时将serviceAccountPermissions中的权限合并到新的ServiceAccount中。ServiceAccount和相关的RBAC资源需要在Pod创建之前准备完成，并且打上各个匹配上的SIdecarSet的finalzier
  + 一个定时运行协程删除没有任何finalizer的serviceAccount和RBAC资源，不跟随pod生命周期。删除前确认没有任何Pod在使用

### 毕业标准
+ Alpha：具有将 ServiceAccount 注入 sidecar 容器的基本功能。
+ Beta：稳定性改进和扩展的验证测试。
+ 稳定：全面的用户反馈已纳入，功能准备好供一般使用。

### 缺点
1. 如果多个 SidecarSets 需要 ServiceAccount 注入，可能会发生冲突。
2. 可能需要额外的用户教育来配置 sidecar 的独立 ServiceAccounts。

### 替代方案
+ 在 SidecarSet 之外手动配置 ServiceAccount 权限。