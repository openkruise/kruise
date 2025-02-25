---
title: 镜像预热支持预热vk
authors:
  - "@Abner-1"
reviewers:
  - "@YYY"
creation-date: 2024-12-04
last-updated: 2024-12-06
status: experimental
see-also:
---

# 镜像预热相关问题

## Summary

镜像预热有几个场景需要进行优化：
1. 和 p2p 任务的配合
   1. 用户可能希望必须要在 p2p 开启的情况下进行预热：不能保证新节点上预热任务会在 p2p ready 之后才开始
   2. p2p 场景其实不希望全局同时预热太多镜像，多个镜像同时/交叉预热可能会降低缓存命中率
2. 新节点预热的时候会因为大量镜像预热会导致其他应用因为iops/网络等资源竞争影响业务
   1. 限制一个节点上同时预热的镜像数 -- 可以缓解，但极端场景可能仍会有影响：docker pull 单个镜像的话都可能打满 iops 影响其他业务是不是也是不合理的
   2. 利用 cgroup v2 来限制 daemon，进而限制子进程的 iops -- cgroup v2 目前应用范围还不够广，且无法根据节点情况自适应调整


## Motivation

镜像预热作为一个有效加速应用启动的特性，在生产环境中有广阔的应用场景；但在实际使用中，我们收到了一些镜像预热对节点/其他组件的影响反馈，如何优化镜像预热场景的可用性和易用性是完善这个解决方案的重要一环。

### Goals

- 优化和 p2p 解决方案的配合机制，增加联动配置以便镜像预热 + p2p 更好地组合使用
- 优化新节点预热场景，增加配置限制预热导致的资源抢占（或以最佳实践的方式提供解决方案）
- 目前只在启动时check p2p ready 状态

### Non-Goals/Future Work

- 暂不在非启动状态 check p2p ready 状态，如预热到一半 p2p 组件挂了，暂不终止预热任务

## Proposal

### 如何感知 p2p 组件已经 ready？
   
   开放配置允许用户配置只在 p2p ready 的情况下进行预热
   - p2p 组件是 pod / 容器部署的，使用 CRI 接口来获取容器相关状态（非容器部署大概率是在容器之前ready）
   - 监听 node status 中的 condition 来查看 p2p 的状态 （用户需要自行写入对应的 condition）
   - 其他方式...

### 如何控制全局同时预热的镜像个数？
1. 使用 ImageListPullJob 
   - 理论上可以控制预热的顺序，等待前一个镜像预热完成后再预热第二个
   - 缺点是不同的用户可能下发多个 ImageListPullJob，如果整个集群只使用一个 ImageListPullJob 也就失去了小范围进行预热的灵活性

   可以在 manager 中通过控制 ImagePullJob 的并发度来实现，虽然可以有更多的粒度来控制，但可能过于复杂了


### 新节点预热大量镜像影响正在运行的业务

1. 大量预热完成之前是否可以视为节点不可用
   这种将预热任务看成节点初始化的一个步骤，虽然不能完美的解决所有场景下预热对节点其他业务的影响，但排除了新节点预热这个热点场景，理论上是可以达到目标的；但其代价是 ecs 从创建到 ready 的时间会加长，节点弹性的速度会降低。

   但如果业务本身镜像就比较庞大，镜像的拉取时长无非是从节点 ready 之后创建 pod 时转移到了 节点 ready 之前。如果预热了多个镜像的话，多花的时间是用在预热别的镜像上了。

2. 预热的时候希望能限制资源使用从而不影响正在运行的业务
   1. 限制一个节点上同时预热的镜像对某些场景上是有效的
   2. 如果能使用 cgroup v2 来限制磁盘iops/带宽的话也许是一个更完善的解决方案，但这个需求值可能不是很好设置（支持百分比设置是不是更好）

目前感觉方案1 不是很适合，方案2-1可以作为一个配置项，长期来看 cgroup v2 来限制是更好的方式。

### Implementation Details

1. daemon config 中增加一个配置项，允许用户配置校验 p2p 组件的方式，包括pod/容器名，或者condition key。

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kruise-daemon-configuration
  namespace: kruise-system
data:
  "Daemon_Check_P2P_Ready": |
    {
      "type": "pod", // condition / container
      "namePrefix": "p2p-plugin", // required when type is pod or container
      "nodeCondition": "p2p-ready", // required when type is condition
    }
```

2. manager 中增加 ImagePullJob 的并发控制，允许用户传入同时最多预热的 ImagePullJob 的个数

3. daemon 中增加一个配置项，允许用户指定 daemon 同时预热的镜像最大个数
如： `--max-image-pull-worker=2`


## Additional Details

### Test Plan [optional]

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR

