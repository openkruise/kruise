# 20260414-ConfigMapSet-enhance-zh

# ConfigMapSet设计

# 背景

在 大量数据/模型 加载场景下，为提高迭代速度，原地升级目前已经成为我们的基本能力，当前主要使用cloneset。

在我们的业务场景中，模型应用更新时主要为数据版本更新，当前通过Env进行控制。

由于数据版本和代码不兼容时，可能需要通过更新镜像进行修复；数据版本在此过程中期望不变化，从而保持数据版本验证进度及不同数据版本仍可提供服务。

期望能够有类似 EnvSet/ConfigMapSet 的控制器，在cloneset之外旁路的管理多版本Env/配置文件，从而实现镜像发布与配置发布解耦

[https://github.com/openkruise/kruise/issues/1894](https://github.com/openkruise/kruise/issues/1894)

# 基本能力

1.  版本管理
    
2.  容器选择
    
3.  reload容器注入
    
4.  更新策略
    
5.  回滚
    

## spec示例

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms
  namespace: my-namespace
spec:
  # 根据标签选择Pod实例
  selector:
    matchLabels:
      app: sample
  # 需要更新的业务配置
  data: 
    application.yaml: |
      server:
        port: 6666
      log:
        level: info
    db.conf: |
      url: "mysql://localhost:3306"
  # 需要被更新的业务容器
  containers:
  - name: main
    mountPath: /data/conf1 # 用于描述挂载路径及可读权限
  - nameFrom:
	 fieldRef:
	   apiVersion: v1
	   fieldPath: metadata.labels['cName']
    mountPath: /data/conf3
  # 用于更新配置文件的容器，在Pod创建时注入
  reloadSidecarConfig:
    # k8s直接注入的方式
    type: k8s
    config:
      name: reload-sidecar
      image: openkruise/reload-sidecar:v1.0.0
      restartPolicy: Always
    # 通过引用Openkruise SidecarSet的方式注入
    type: sidecarset
    config:
      # 指定SidecarSet中的container
      sidecarSetRef:
        name: reload-sidecarSet
        containerName: reload-sidecar
    # 自定义容器注入
    type: custom
    config:
      # 通过ConfigMap指定reload-sidecar配置, 方便跨namespace统一配置
      configMapRef:
        name: reload-configMap
        namespace: default
  effectPolicy:
    # reload-sidecar和业务容器都会重启，reload-sidecar先于业务容器重启
    type: ReStart
    # 仅reload-sidecar容器重启，重启完成后，执行HTTP/TCP触发业务容器更新配置
    type: PostHook
    postHook:
      httpGet:
        port: 8081
        path: /reload-config
      tcpSocket:
        port: 8081
    # 不重启任何容器，reload-sidecar容器会自动根据ConfigMap热更新，并解析出来最新的配置后，共享给业务容器，业务容器再根据共享文件更新后热加载。
    type: HotUpdate
  revisionHistoryLimit: 5
  updateStrategy:
    partition: 10% # 用于灰度滚动更新
    maxUnavailable: 1
    # 用于对匹配的Pod实例分组，分组内的实例按照partition和maxUnavailable的语义滚动更新，不设置默认所有Pod实例为一个分组
    matchLabelKeys:
      - app-group
status:
  currentCustomVersion: v1.0
  currentRevision: e398776e45b31aef
  expectedUpdatedReplicas: 18
  observedGeneration: 8
  readyReplicas: 20
  replicas: 20
  updateCustomVersion: v1.1
  updateRevision: ee70811c764c4bf6
  updatedReadyReplicas: 18
  updatedReplicas: 18
```

## 版本管理

为每一个ConfigMapSet创建一个关联的ConfigMap(RevisionManager-ConfigMap后续将简称为RMC)，持久化ConfigMapSet的多版本数据。revision 建议按 hash 命名，hash 相同视为 no-op，不产生新 revision。

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: deploy-cms1-hub
  namespace: infra-demo-uat
data: 
  revisions: |
    en3kp9:
      settings.yaml: |
        value: aaa
    fes34f:
      settings.yaml: |
        value: bbb
```

ConfigMapSet版本更新时，ConfigMapSet控制器会为需要更新的pod注入Annotation configMapSet/cms1/Revision，用以描述当前配置数据版本。

revisionHistoryLimit用以描述最多在RMC中维护多少个版本的数据

```yaml
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

上述的例子中，当revisionHistoryLimit=5时，修改ConfigMapSet data时webhook会判断，是否为历史某个版本的配置，如果是，移动历史配置文件到最前面。如果不是，则判断最旧的配置版本是否还有Pod在使用，如果有，拒绝这次变更，如果没有，则删除旧版本配置，在最前面插入新配置。

~~对于配置文件的管理方式，默认使用上述ConfigMap Hub的形式管理，对于超过1MB大配置文件的需求，支持通过扩展接口，使用方可以实现该接口（例如：Read/Write）来接入。~~

## 容器选择

容器选择决定了关联的业务容器

### 静态container注入

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  containers:
  - name: main
    mountPath: /data/conf1 
```

### 动态container注入

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  containers:
  - nameFrom: // 在绑定多个负载时，可以通过标签用为不同的容器挂载配置
     fieldRef:
       apiVersion: v1
       fieldPath: metadata.labels['cName']
    mountPath: /data/conf3 
```

## Reload Sidecar容器注入

Reload sidecar容器配置了注入到Pod中用于自动更新配置文件的Container

### 显示声明reloadContainer注入

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  reloadSidecarConfig:
    # k8s直接注入的方式
    type: k8s
    config:
      name: reload-sidecar
      image: openkruise/reload-sidecar:v1.0.0
      restartPolicy: Always

```

该种方式会直接在Pod创建的时候，通过webhook注入Container的方式注入到Pod中，并自动生成emptyDir volume和InitContainer volumeMount，同时业务容器也会注入相应的volumeMount。该container注入时，会启用apps.kruise.io/container-launch-priority: Ordered注解，用于保证先于业务容器启动。

### 引用SidecarSet注入声明

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  reloadSidecarConfig:
    # 通过引用Openkruise SidecarSet的方式注入
    type: sidecarset
    config:
      # 指定SidecarSet中的container
      sidecarSetRef:
        name: reload-sidecarSet
        ContainerName: reload-sidecar
```

该模式下，SidecarSet的Selector需要覆盖ConfigMapSet的Selector，Pod在创建时会先执行SidecarSet的注入，再接着执行ConfigMapSet的注入，SidecarSet负责注入Container，而ConfigMapSet负责注入相关的volumeMount和Env。该模式下的reload-sidecar可以具备横向灰度更新的能力，该能力完全由SidecarSet实现，满足需要灰度更新reload-sidecar镜像版本的需求。

### 通过ConfigMap自定义注入reloadContainer

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
    # 自定义容器注入
    type: custom
    config:
      # 通过ConfigMap指定reload-sidecar配置, 方便跨namespace统一配置
      configMapRef:
        name: reload-configMap
        namespace: default
```

通过ConfigMap配置Reload容器。使用该模式，可以更方便的为不同namespace下的Pod提供统一的reload-sidecar配置。

#### 自定义ConfigMap示例

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: reload-configMap
  namespace: default
data: |
   {
     name: reload-sidecar
     image: openkruise/reload-sidecar:v1.0.0
     restartPolicy: Always
     command: ['sh', '-c', 'tail -F /opt/logs.txt']
   }
```

## 配置更新策略

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  effectPolicy:
    # reload-sidecar和业务容器都会重启，reload-sidecar先于业务容器重启
    type: ReStart
    # 仅reload-sidecar容器重启，重启完成后，执行HTTP/TCP触发业务容器更新配置
    type: PostHook
    postHook:
      httpGet:
        port: 8081
        path: /reload-config
      tcpSocket:
        port: 8081
  updateStrategy:
    partition: 10% # 用于灰度滚动更新
    maxUnavailable: 1
    # 用于对匹配的Pod实例分组，分组内的实例按照partition和maxUnavailable的语义滚动更新，不设置默认所有Pod实例为一个分组
    matchLabelKeys:
      - app-group
```

在ConfigMapSet data更新后，控制器执行流程如下：

1.  更新RMC数据
    
2.  Pod更新Annotation：apps.kruise.io/configmapset.updateRevision的值为当前值
    
3.  根据effectPolicy策略，进行配置生效
    
    1.  ReStart（默认）：
        
        1.  先重启reload-sidecar容器
            
        2.  等待reload-sidecar容器就绪
            
        3.  检查是否已经加载最新配置
            
        4.   重启业务容器。
            
        5.  等待业务容器就绪
            
    2.  HotUpdate: 
        
        1.  Reload容器和业务容器都不会重启，Reload容器会监听ConfigMap文件的更新，自动热更新EmptyDir中的配置文件。
            
        2.  检查是否已经加载最新配置。
            
        3.  业务侧容器需要自行实现对配置文件的热加载。
            
    3.  PostHook：
        
        1.  先重启reload-sidecar容器
            
        2.  等待Reload容器重启就绪
            
        3.  检查是否已经加载最新配置。
            
        4.  通过effectPolicy.postHook定义的方式通知到业务容器，触发业务容器更新配置文件。支持TCP、HTTP的方式。
            
4.  更新ConfigMapSet Status
    

### 滚动策略:

partition 的语义是 **保留旧版本 Pod 的数量或百分比**，默认为 0。这里的 partition **不表示**任何 order 序号。 maxUnavailable 的语义是 **滚动更新过程中最大不可用的 Pod 的数量或百分比。**

根据滚动策略，每次不会超过maxUnavailable个Pod被同时更新，总的更新数量不会超过replicas - partition

当新Pod创建时，获取当前分组下，新旧版本的信息，再根据Partition语义决定当前Pod是用新版本还是旧版本，然后注入相应的信息。

### Pod选择策略

Controller依据partition挑选更新Pod时，为了保证版本数量收敛趋势，会按照如下规则优先更新：

1.  非currentRevision > currentRevision
    
2.  非currentRevision序号小的版本 > 非currentRevision序号大的版本
    
3.  非currentRevision不存在于RMC的版本 > 非currentRevision存在于的版本
    

### 启动顺序

在重建情况下，需要保证Reload容器一定需要比业务容器先启动成功，这里会使用openkruise的Container Launch Priority 特性**控制一个 Pod 中容器的启动顺序**。

### 重建生效场景

对于ConfigMapSet中非配置文件（data部分）的更新，无论是什么更新策略，都不会做任何处理，需要等到业务侧下次部署时Pod重建后生效。

### 叠加发布

当更新到一半的时候，重新发起了一次新的更新，这个时候会重新执行一次上述更新流程，Pod选择策略会优先更新不是currentRevision的Pod

### 卸载流程

考虑到k8s Pod的自愈能力，这里不能直接删除ConfigMapSet，避免在Pod迁移/重建时，因为缺少ReloadContainer的注入，存量实例异常。这里更优雅的方式是通过finalizer卡住ConfigMapSet的删除流程，这个时候控制器不再对新Pod进行注入，直到业务侧自行完成一次全量的Workload更新后，ConfigMapSet不再作用于任何Pod，这个时候控制器进行清理工作（清理ConfigMap），最后移除finalizer，完成卸载

## 回滚流程

直接重新Apply ConfigMapSet，控制器会重新按照上述滚动更新流程执行一次，将配置文件再一次更新回原来的版本。无论是回滚到currentRevision还是historyRevision，都可以通过Apply ConfigMapSet的方式回滚。 对于回滚到currentRevision，存量为更新的Pod不会动，会根据Pod选择策略对非currentRevision更新。对于historyRevision，等同于上述完整的发布流程。

## 场景限制

1.  不支持注入多个Reload容器
    
2.  不支持多个ConfigMapSet控制同一个Pod下的同一个path
    
3.  暂不支持配置和业务Spec同时修改的场景
    
4.  不支持virtual-kubelet节点上的Pod热更新