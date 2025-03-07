# ConfigMapSet设计

# 背景

在 大量数据/模型 加载场景下，为提高迭代速度，原地升级目前已经成为我们的基本能力，当前主要使用cloneset。

在我们的业务场景中，模型应用更新时主要为数据版本更新，当前通过Env进行控制。

由于数据版本和代码不兼容时，可能需要通过更新镜像进行修复；数据版本在此过程中期望不变化，从而保持数据版本验证进度及不同数据版本仍可提供服务。

期望能够有类似 EnvSet/ConfigMapSet 的控制器，在cloneset之外旁路的管理多版本Env/配置文件，从而实现镜像发布与配置发布解耦

[https://github.com/openkruise/kruise/issues/1894](https://github.com/openkruise/kruise/issues/1894)

# 基本能力

1. 版本管理

2. configmap管理

3. 容器选择

4. 更新策略

5. 执行顺序

## spec示例

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  selector:
    matchLabels:
      app: sample
  data: 
    settings.yaml: |
	  value: aaa
  containers:
  - name: main
    mountPath: /data/conf1 // 用于描述挂载路径及可读权限
  - name: sidecar1
    mountPath: /data/conf2
  - nameFrom: // 为兼容动态注入场景(例如sidecarset)提供路径
	  fieldRef:
	    apiVersion: v1
	    fieldPath: metadata.labels['cName']
     mountPath: /data/conf3
  revisionHistoryLimit: 5
  // injectUpdateOrder: true 和workload控制器同时操作时的机制
  updateStrategy:  
    partition: 10% // 用于版本管理
    restartInjectedContainers: true
    maxUnavailable: 1
  injectStrategy:
    pause: true
status:
  observedGeneration: 2
  currentRevision: en3kp9
  updateRevision: fes34f
  matchedPods: 3
  updatedPods: 1
  readyPods: 3
  updatedReadyPods: 1
  lastContainersTimestamp: "2025-02-14T17:51:48Z"

```

## 版本管理

为每一个ConfigMapSet创建一个独特的ConfigMap(RevisonManager-ConfigMap后续将简称为RMC)，持久化ConfigMapSet的多版本数据，并严格按照顺序(如果新版本与历史版本hash相同将引起顺序变化)

```plaintext
apiVersion: v1
kind: ConfigMap
metadata:
  name: deploy-cms1-hub
  namespace: infra-demo-uat
spec:
  data: 
    revisions: |
      en3kp9:
        settings.yaml: |
          value: aaa
      fes34f:
        settings.yaml: |
          value: bbb

```

ConfigMapSet版本更新时，ConfigMapController会为需要更新的pod注入Annotation configMapSet/cms1/Revision，用以描述当前配置数据版本。

revisionHistoryLimit用以描述最多在RMC中维护多少个版本的数据

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  # ...
  revisionHistoryLimit: 5
  # ...

```

## 配置管理

通过admission webhook为被托管Pod注入emptyDir声明，并以spec中描述的规则为container注入volumeMounts。

通过admission webhook为被托管Pod注入特殊的sidecar(后续称为reload-sidecar)，reload-sidecar会挂载RMC及通过envFromMetaData注入pod annotation configMapSet/cms1/Revision所描述的配置版本。

reload-sidecar负责解析RMC数据并在emptyDir中维护需要的配置版本

## 容器选择

容器选择决定了托管的容器，以及动态更新时会触发重启的容器

### 静态container注入

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec
  # ...
  containers:
  - name: main
    mountPath: /data/conf1 

```

### 动态container注入

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  containers:
    - nameFrom: // 为兼容动态注入场景(例如sidecarset)提供路径
	  fieldRef:
	    apiVersion: v1
	    fieldPath: metadata.labels['cName']
      mountPath: /data/conf4

```

## 更新策略

需更新Pod成功被更新Annotation configMapSet/cms1/Revision后，Kruise Daemon会负责触发reload-sidecar进行重启，从而加载最新版本

updateStrategy用于描述更新比例具体的更新策略

partition 的语义是 **保留旧版本 Pod 的数量或百分比**，默认为 0。这里的 partition **不表示**任何 order 序号。

PS: 需要支持partition增大feature以覆盖az回滚场景

Controller依据partition挑选更新Pod时，为了保证版本数量收敛趋势，会按照如下规则优先更新：

1. 非currentRevision > curentRevision

2. 非currentRevision序号小的版本 > 非currentRevision序号大的版本

3. 非currentRevision不存在于RMC的版本 > 非currentRevision存在于的版本

### 仅注入(默认策略)

reload-sidecar重启完成后，Kruise Daemon将Pod状态更新并触发后续Controller行为。

injectUpdateOrder开启后，Controller将在选定需重启pod后，为pod加上label configMapSet/cms1/updateOrder=1，为控制面/kruise用户提供异步更新pod的标志，Controller会在watch到pod被注入配置的容器重启后，移除该label。

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  # ...
  injectUpdateOrder: true
  # ...

```

对应workload应该由平台控制面/kruise用户进行配合，例如注入priorityStrategy

```plaintext
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    priorityStrategy:
      orderPriority:
        - orderedKey: configMapSet/cms1/updateOrder

```

### 重启注入配置容器

restartInjectedContainers开启后，Kruise Daemon会在reload-sidecar重启完后，根据最新的ConfigMapSet注入配置的容器进行重启，并在重启完成后再修改pod状态。

maxUnavailable用于在restartInjectedContainers开启场景下描述最大不可用的pod数

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  # ...
  injectUpdateOrder: true
  updateStrategy:  
    partition: 10% // 用于版本管理
    restartInjectedContainers: true
    maxUnavailable: 1

```

## 启动顺序

同时更新/启动时reload-sidecar一定需要比配置注入容器先重启/启动

a. K8S 版本<1.28，OpenKruise Container Launch Priority

b. K8S 版本>=1.28，可使用k8s initContainer restartPolicy=Always

(?)触发restartInjectedContainers时，需要根据容器启动顺序进行重启

## 冷启动

mount path变更时，需要实现冷启动，即pod重建后生效，但ConfigMapSetController本身并不触发pod漂移

# 实现

## spec定义

```plaintext
// ConfigMapSetSpec定义期望状态
type ConfigMapSetSpec struct {
  Selector           *metav1.LabelSelector      `json:"selector"`
  Data               map[string]string          `json:"data"`
  Containers     []ContainerInjectSpec      `json:"containers"`
  RevisionHistoryLimit *int32                   `json:"revisionHistoryLimit,omitempty"`
  InjectUpdateOrder  bool                       `json:"injectUpdateOrder,omitempty"`
  UpdateStrategy     ConfigMapSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type ContainerInjectSpec struct {
  Name     string            `json:"name,omitempty"`
  NameFrom *ValueFromSource  `json:"nameFrom,omitempty"`
  MountPath string            `json:"mountPath"`
}

type ValueFromSource struct {
  FieldRef corev1.ObjectFieldSelector `json:"fieldRef"`
}

type ConfigMapSetUpdateStrategy struct {
  Partition               *string `json:"partition,omitempty"`
  RestartInjectedContainers bool   `json:"restartInjectedContainers,omitempty"`
  MaxUnavailable          *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// ConfigMapSetStatus定义观测状态
type ConfigMapSetStatus struct {
  ObservedGeneration int64  `json:"observedGeneration,omitempty"`
  CurrentRevision    string `json:"currentRevision,omitempty"`
  UpdateRevision     string `json:"updateRevision,omitempty"`
  MatchedPods        int32  `json:"matchedPods"`
  UpdatedPods        int32  `json:"updatedPods"`
  ReadyPods          int32  `json:"readyPods"`
  UpdatedReadyPods   int32  `json:"updatedReadyPods"`
  LastContainersTimestamp *metav1.Time `json:"lastContainersTimestamp"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ConfigMapSet struct {
  metav1.TypeMeta   `json:",inline"`
  metav1.ObjectMeta `json:"metadata,omitempty"`

  Spec   ConfigMapSetSpec   `json:"spec,omitempty"`
  Status ConfigMapSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ConfigMapSetList struct {
  metav1.TypeMeta `json:",inline"`
  metav1.ListMeta `json:"metadata,omitempty"`
  Items           []ConfigMapSet `json:"items"`
}


```

## Pod状态标志位定义

通过Pod Annotation来描述ConfigMapSet版本状态：

**apps.kruise.io/configMapSet/:configMapSet名称/revision**: 描述预期版本。会由Controller在更新Pod时patch

**apps.kruise.io/configMapSet/:configMapSet名称/revisionTimestamp**: 描述预期版本的最后更新时间。会由Controller在更新Pod patch时一并更新

**apps.kruise.io/configMapSet/:configMapSet名称/currentRevision**: 描述当前版本。会由Kruise Daemon在reload-sidecar重启完成(若开启restartInjectedContainers，则需要等注入配置容器全量重启)后patch对应Pod

**apps.kruise.io/configMapSet/:configMapSet名称/currentRevisionTimestamp**: 描述currentRevision的最后更新时间。会由Kruise Daemon在reload-sidecar重启完成(若开启restartInjectedContainers，则需要等注入配置容器全量重启)后patch对应Pod时一并操作

## Controller核心逻辑

### Reconcile

```plaintext
// 核心协调逻辑
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
  // 获取ConfigMapSet实例

  // 处理删除逻辑

  // 冷启动状态处理
  
  // 版本管理

  // Pod同步

  // Pod版本状态计算
  
  // 更新状态

}


```

#### 删除逻辑

删除configmapset，但是删除不清理/重建Pod

#### 冷启动状态变更

**status.lastContainersTimestamp**: 最后containers更新时间更新时间，用于冷启动决策

**status.lastContainersHash：**

当spec.containers变更时，需要对新旧版本做hash，如果hash变更则在更新Pod时更新lastContainersTimestamp。并reload当前configMapSet对象

#### 版本管理

```plaintext
// 版本管理逻辑
func (r *Reonciler) manageRevisions(ctx context.Context, cms *apsv1alpha1.ConfigMapSet) (string, error) {
  // 计算当前Spec的Hash

  // 获取或创建/更新RevisionManager ConfigMap

}

```

#### Pod同步

```plaintext
// Pod同步逻辑
func (r *econciler) syncPods(ctx context.Context, cms *ppsv1alpha1.ConfigMapSet) error {
  // 获取关联Pod
  
  // 冷启动判断
  
  // 根据更新策略选择要更新的Pod

  // 更新选中的Pod

}


```

##### 冷启动

当前spec.containers变更时，不会触发Pod重建，当Pod新建时才会按新的containers生效。mount相关注入无法动态生效，故存量Pod（pod创建时间小于status.lastContainersTimestamp）不进行configMapSet版本变更行为

##### Pod选择策略

Controller依据partition挑选更新Pod时，筛掉已更新Pod后，为了保证版本数量收敛趋势，会根据Annotation的标志位按照如下规则优先更新：

1. 非currentRevision > currentRevision

2. 非currentRevision不存在于RMC的版本 > 非currentRevision存在于的版本

3. 非currentRevision序号小的版本 > 非currentRevision序号大的版本

##### Pod更新

更新Pod Annotation **apps.kruise.io/configMapSet/:configMapSet名称/revision**

#### Pod状态更新

根据Pod Container状态和Annotation计算Pod的CMS相关状态并更新至Annotation

#### 状态更新

```plaintext
// 状态更新逻辑
func (r Reconciler) updateStatus(ctx context.Context, cms appsv1alpha1.ConfigMapSet, newRevision string) error {
  // 获取关联Pod列表

  // 计算状态指标

  // 更新ConfigMapSet状态
}

```

**status.currentRevision**: ConfigMapSet当前版本hash

**status.updateRevision**: ConfigMapSet更新版本hash

**status.matchedPods**: 关联Pod数量

**status.updatedPods**: 更新完成Pod数量

**status.readyPods**: 健康Pod数量

**status.updatedReadyPods**: 健康且更新完成Pod数量

当Pod Annotation 

configMapSet/cms1/currentRevision=configMapSet/cms1/revision时，视为Pod更新完成 

## Webhook注入逻辑

```plaintext
// +kubebuilder:webhook:path=/mutate-v1alpha1-configmapset,mutating=true,failurePolicy=fail,groups=apps.kruise.io,resources=configmapsets,verbs=create;update,versions=v1alpha1,name=mconfigmapset.kb.io

// 实现volume和sidecar注入
func (h *ConfigMapSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
  // 1. 是否需要emptyDir volume及添加

  // 2. 是否需要reload-sidecar容器及添加

  // 3. 是否需要volumeMounts到目标容器spec及添加

  // 4. 根据K8S版本设置启动顺序

  // 5. 根据partition注入版本Annotation
  
}


```

同时更新/启动时reload-sidecar一定需要比配置注入容器先重启/启动

a.K8S 版本<1.28，OpenKruise Container Launch Priority

b.K8S 版本>=1.28，可使用k8s initContainer restartPolicy=Always

## Daemon协调逻辑

### 重启容器

```plaintext
// pkg/daemon/containermeta/container_meta_controller.go
func (c *Controller) manageContainerMetaSet(pod *v1.Pod, kubePodStatus *kubeletcontainer.PodStatus, oldMetaSet *appspub.RuntimeContainerMetaSet, criRuntime criapi.RuntimeService) *appspub.RuntimeContainerMetaSet {
// ...
// 监控Pod Annotation变化，触发reload-sidecar重启

// 重启injectedContainers
// ...
}

func eventFilter(oldPod, newPod *v1.Pod) bool {
// ...

// 增加configMapSet注入筛选逻辑

// ...
  return
}

```

当容器重启时间小于Anno revisionTimestamp，revision!=updateRevision时，认为容器需要重启。

#### 重启injectedContainers

需要加载ConfigMapSet，获取spec.containers进行解析，从而确定需要重启的Container。

值得注意的的是，有可能存在异步请求到spec.containers已经变更的情况，故而需要根据status.lastContainersTimestamp进行判定如果Pod创建时间小于lastContainersTimestamp，则不进行后续重启动作

### 更新Pod状态标志

```plaintext
// pkg/daemon/containerrecreate/crr_daemon_controller.go
func (c *Controller) manage(crr *appsv1alpha1.ContainerRecreateRequest) error {
// ...
// 更新Pod状态Annotation
// ...
}

```

当Anno revisionTimestamp<所有需要重启的Container的重启时间时，且Pod状态为ready时，认为Pod配置更新完成，更新Anno currentRevision至revision,Anno currentRevisionTimestamp更新到当前时间

# 与平台控制面协作

## 原地升级场景

### 仅常规/镜像发布场景

正向：ConfigMapSet更新并指定customVersion，updateStrategy.upgradeType=ColdUpgrade,对应customVersion的distribution按滚动节奏更新

回滚：对应customVersion的distribution按回滚节奏更新，最后更新ConfigMapSet.Spec至老版本

![流程图1](https://github.com/user-attachments/assets/8166de4f-c538-42e7-9769-4765b9178636)

### 仅配置发布场景

#### 正向

ConfigMapSet更新并指定customVersion，指定updateStrategy.upgradeType=HotUpgrade和maxUnavailable，从而触发容器重启；对应customVersion的distribution按滚动节奏更新

#### 回滚

指定updateStrategy.upgradeType=HotUpgrade和maxUnavailable，从而触发容器重启；对应customVersion的distribution按回滚节奏更新，最后更新ConfigMapSet.Spec至老版本

![流程图2](https://github.com/user-attachments/assets/53390e50-acc1-4887-a472-a13a1451aa6d)


### 常规/镜像发布+配置发布场景(?)

#### 正向

ConfigMapSet更新并指定customVersion新增版本，updateStrategy.upgradeType=ColdUpgrade,对应customVersion的distribution按滚动节奏更新;先变更ConfigMapSet，后更新workload

#### 回滚

对应customVersion的distribution按回滚节奏更新，最后更新ConfigMapSet.Spec至老版本;先变更ConfigMapSet，后更新workload

![流程图3](https://github.com/user-attachments/assets/f94f56f6-5a7b-4f84-acf2-bd65f572a11b)
