# ConfigMapSet 增量变化提案说明

## 文档目的

本文对比以下两版提案，输出一份面向评审的增量变化说明，重点突出新提案相较旧提案的设计改进点、语义收敛点与落地价值。

- 旧提案：`https://github.com/openkruise/kruise/pull/2408/changes#diff-e33f815106047a3386f45fa8b743f73d22537670b79b46e0173c297f87ee7288`
- 新提案：`https://github.com/openkruise/kruise/pull/2408/changes#diff-ed8540e7f9763bfd9d83c178411aa64b7e43196890d77c20e93d19c2e416ac19`

本文不重复完整方案，仅描述“新增、调整、收敛、删除”的关键变化。

---

## 一、总体结论

新提案相较旧提案，最大的变化不是“补充更多实现细节”，而是将方案从“偏控制器内部实现设计”升级为“面向用户可配置能力的产品化方案”。整体改进体现在：

1. **能力抽象更清晰**：从“ConfigMap 管理 + sidecar 注入 + 重启逻辑”拆解为“版本管理、容器选择、Reload Sidecar 注入、配置生效策略、回滚”等更清晰的能力模块。
2. **用户接口更完整**：新增 `reloadSidecarConfig`、`effectPolicy`、`matchLabelKeys` 等关键字段，使方案从单一路径变成可组合的多模式能力。
3. **配置生效方式更丰富**：从旧提案基本聚焦“重启生效”，扩展为 `ReStart`、`PostHook`、`HotUpdate` 三种生效模型，覆盖更多业务场景。
4. **发布与回滚闭环更完整**：新提案显式补齐回滚流程、叠加发布、卸载流程与场景限制，评审可读性和落地指导性更强。
5. **平台协作边界更合理**：旧提案中很多与 Daemon、Webhook、Pod Annotation、冷启动判断强绑定的实现细节被弱化，转而强调 CRD 语义与控制面行为，降低了设计阶段的耦合度。

---

## 二、核心改进点

## 1. Reload Sidecar 从“固定注入”升级为“多模式注入”

### 旧提案

旧提案默认通过 admission webhook 注入一个特殊 sidecar（reload-sidecar），方案基本只有一条路径：

- 注入 emptyDir
- 注入 reload-sidecar
- reload-sidecar 解析 RMC
- 通过重启 sidecar / 业务容器使配置生效

该方式可行，但灵活性不足。

### 新提案改进

新提案新增 `reloadSidecarConfig`，显式支持三类注入模式：

1. `type: k8s`
   直接由 ConfigMapSet 在 Pod 创建时注入容器。

2. `type: sidecarset`
   通过引用 OpenKruise `SidecarSet` 中的容器完成注入。

3. `type: custom`
   通过引用 `ConfigMap` 自定义 reload-sidecar 配置，实现跨命名空间复用。

### 改进价值

- **注入方式可配置**：不再强制只有 webhook 直注一种模式。
- **复用已有 SidecarSet 能力**：天然支持 sidecar 镜像独立灰度、独立演进。
- **更适配平台化场景**：支持统一维护 reload 容器模板，降低不同业务线重复配置成本。
- **降低耦合度**：ConfigMapSet 不再必须完全接管 reload-sidecar 的全部细节。

---

## 2. 配置生效策略从“以重启为核心”升级为“三种 effectPolicy”

### 旧提案

旧提案的核心生效策略主要围绕：

- reload-sidecar 重启
- 可选重启被注入配置的业务容器
- 通过 `restartInjectedContainers` 和 `maxUnavailable` 控制滚动更新

本质上是“以重启驱动配置生效”。

### 新提案改进

新提案引入 `effectPolicy`，支持三种配置生效模式：

1. `ReStart`
   - 先重启 reload-sidecar
   - 再重启业务容器
   - 适用于最稳妥的强一致场景

2. `PostHook`
   - reload-sidecar 重启并准备好新配置
   - 通过 HTTP/TCP 回调通知业务容器加载配置
   - 适用于业务已有 reload 接口，但不希望业务容器重启的场景

3. `HotUpdate`
   - Reload 容器与业务容器都不重启
   - reload-sidecar 热更新共享配置文件
   - 业务容器自行实现热加载
   - 适用于对中断极度敏感的在线场景

### 改进价值

- **能力覆盖面显著扩大**：从单一“重启生效”升级为“重启 / 通知 / 热更新”三模型并存。
- **贴近真实业务差异**：不同业务的配置加载机制不同，新提案不再强迫统一路径。
- **降低发布扰动**：`PostHook` 与 `HotUpdate` 为低抖动场景提供了明确支持。
- **更利于配置发布与镜像发布解耦**：配置更新可独立选择生效方式，而不是绑定容器重启策略。

---

## 3. 更新策略进一步增强，支持分组滚动

### 旧提案

旧提案的滚动更新主要依赖：

- `partition`
- `maxUnavailable`
- `restartInjectedContainers`
- `injectUpdateOrder`

其中 `injectUpdateOrder` 需要业务 workload 或平台控制面配合 priorityStrategy 使用，存在一定协作复杂度。

### 新提案改进

新提案保留并强化了滚动语义，同时新增：

- `matchLabelKeys`

用于按标签对匹配 Pod 分组，组内分别执行 `partition` / `maxUnavailable` 的滚动策略。

### 改进价值

- **灰度能力更细粒度**：不是对全量 Pod 做一个统一滚动窗口，而是可以按分组独立推进。
- **更适合多 AZ / 多分片 / 多租户场景**：例如按 `app-group`、机房标签、业务分区等分组控制节奏。
- **减少平台外部依赖**：旧提案依赖 `injectUpdateOrder + workload priorityStrategy` 的协作方式，在新提案中弱化为更直接的控制器分组滚动能力。
- **语义更统一**：更新策略直接体现在 ConfigMapSet 自身 spec 中，减少跨对象联动心智负担。

---

## 4. 回滚能力从“隐含支持”升级为“显式流程设计”

### 旧提案

旧提案虽然提到版本管理、历史版本保留和回滚场景，但回滚流程更多体现在平台协作的流程图和场景讨论中，缺少统一、简洁、直接的方案描述。

### 新提案改进

新提案单独增加“回滚流程”章节，明确：

- 可以直接重新 Apply ConfigMapSet 完成回滚
- 支持回滚到 `currentRevision`
- 支持回滚到 `historyRevision`
- 回滚流程复用正常滚动更新流程
- 存量未更新 Pod 不会被多余扰动，优先处理非 `currentRevision` Pod

### 改进价值

- **回滚路径更清楚**：从“依赖额外平台编排理解”变成“CRD 自身支持的标准流程”。
- **降低运维操作复杂度**：通过重新 Apply 即可回滚，符合 Kubernetes 习惯。
- **与版本管理闭环更紧密**：revisionHistoryLimit、RMC、滚动更新策略与回滚自然联动。
- **提高方案可信度**：配置发布方案没有回滚闭环通常难以落地，新提案补齐了这块短板。

---

## 5. 卸载流程被正式设计，避免 Pod 自愈带来的隐患

### 旧提案

旧提案在删除逻辑中提到：

- 删除 ConfigMapSet 不清理、不重建 Pod

但没有完整设计“如何安全卸载”。

### 新提案改进

新提案新增“卸载流程”设计：

- 使用 `finalizer` 卡住删除
- 控制器停止对新 Pod 注入
- 等待业务侧完成一次全量 workload 更新
- 确认 ConfigMapSet 不再作用于任何 Pod 后
- 再清理关联 ConfigMap 并移除 finalizer

### 改进价值

- **补齐运维生命周期**：不仅考虑“如何创建和更新”，也考虑“如何安全退出”。
- **避免 Pod 自愈异常**：防止 Pod 重建后因失去 Reload 容器或挂载注入而行为不一致。
- **更符合生产要求**：卸载流程明确后，平台才能放心开放给业务使用。

---

## 6. 状态定义更面向发布观测，而不是仅面向控制器内部

### 旧提案

旧提案 Status 主要包括：

- `matchedPods`
- `updatedPods`
- `readyPods`
- `updatedReadyPods`
- `currentRevision`
- `updateRevision`
- `lastContainersTimestamp`

更偏底层控制器状态统计。

### 新提案改进

新提案 Status 演进为：

- `currentCustomVersion`
- `updateCustomVersion`
- `currentRevision`
- `updateRevision`
- `replicas`
- `readyReplicas`
- `updatedReplicas`
- `updatedReadyReplicas`
- `expectedUpdatedReplicas`

### 改进价值

- **更贴近发布视角**：不仅能看 revision，还能看业务可理解的 customVersion。
- **更适合控制面展示**：副本语义与 Kruise / Deployment 的常见观测模型更一致。
- **便于做发布进度判断**：`expectedUpdatedReplicas` 使“期望更新量”更清楚。
- **降低理解门槛**：状态字段更适合面向用户、控制面和运维可视化展示。

---

## 7. 非配置项变更的语义更收敛，避免动态能力被过度承诺

### 旧提案

旧提案专门讨论了“冷启动”：

- `mountPath` 变更需要冷启动
- `spec.containers` 变更通过 `lastContainersTimestamp` 判断
- Pod 创建时间与时间戳对比决定是否参与更新
- Daemon 重启业务容器时还要判断旧容器定义是否仍有效

方案完整，但实现复杂度高，边界也较难解释。

### 新提案改进

新提案直接收敛为：

- **对于 ConfigMapSet 中非 data 部分的修改，不做动态处理**
- 等到业务侧下次部署、Pod 重建后生效

### 改进价值

- **边界更清楚**：只承诺“配置内容更新”的动态能力，不承诺“挂载结构/注入结构变更”的在线生效。
- **实现复杂度更低**：减少对时间戳、Pod 创建时间、存量实例特殊判断的依赖。
- **行为更稳定**：避免在复杂场景中出现控制器与 Daemon 认知不一致。
- **更利于第一阶段落地**：优先保证核心能力可用，再逐步扩展动态变更能力。

这是典型的“能力收敛型优化”，虽然表面上像限制，但实际上提升了方案可落地性。

---

## 8. 方案增加了“叠加发布”和“场景限制”，风险边界更明确

### 旧提案

旧提案虽然讨论了很多流程，但对“半途再次发布怎么办”“明确不支持什么”描述不够集中。

### 新提案改进

新提案显式补充：

- **叠加发布**
  - 更新过程中再次发起新版本变更，会重新走一轮滚动流程
  - Pod 选择优先处理非 `currentRevision`

- **场景限制**
  - 不支持注入多个 Reload 容器
  - 不支持多个 ConfigMapSet 控制同一 Pod 的同一路径
  - 暂不支持配置和业务 Spec 同时修改
  - 不支持 virtual-kubelet 节点上的 Pod 热更新

### 改进价值

- **设计边界更透明**
- **风险预期更可控**
- **后续迭代路线更清晰**
- **评审时更容易判断 MVP 范围**

