package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ConfigMapSetSpec defines the desired state of ConfigMapSet
type ConfigMapSetSpec struct {
	// +optional
	// 当前版本的别名
	CustomVersion string `json:"customVersion,omitempty"`
	// Selector 用来挑选pod
	Selector *metav1.LabelSelector `json:"selector"`
	// Data 存放的是配置与内容的映射
	Data map[string]string `json:"data"`
	// Containers 表示的是需要被注入配置的容器
	Containers []ContainerInjectSpec `json:"containers"`
	// ReloadSidecarConfig 定义了reload容器的注入方式和配置
	// +optional
	ReloadSidecarConfig *ReloadSidecarConfig `json:"reloadSidecarConfig,omitempty"`
	// EffectPolicy 定义了配置更新的生效策略
	// +optional
	EffectPolicy *EffectPolicy `json:"effectPolicy,omitempty"`
	// RevisionHistoryLimit 用于描述最多在RMC中维护多少个版本的数据
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
	// UpdateStrategy 指定更新策略, 详见结构体定义
	// +optional
	UpdateStrategy ConfigMapSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type ContainerInjectSpec struct {
	// +optional
	Name string `json:"name,omitempty"` // 需要挂载配置的容器名(静态注入)
	// +optional
	NameFrom *ValueFromSource `json:"nameFrom,omitempty"` // 从pod中获取(动态注入), 如metadata.labels['cName']
	// MountPath 容器内挂载路径
	MountPath string `json:"mountPath"`
}

type ValueFromSource struct {
	FieldRef corev1.ObjectFieldSelector `json:"fieldRef"`
}

// ReloadSidecarType 定义 ReloadSidecar 注入类型
type ReloadSidecarType string

const (
	// K8sConfigReloadSidecarType 直接在ConfigMapSet中配置
	K8sConfigReloadSidecarType ReloadSidecarType = "k8s"
	// SidecarSetReloadSidecarType 引用外部的SidecarSet
	SidecarSetReloadSidecarType ReloadSidecarType = "sidecarset"
	// CustomerReloadSidecarType 引用自定义ConfigMap
	CustomerReloadSidecarType ReloadSidecarType = "custom"
)

type ReloadSidecarConfig struct {
	Type ReloadSidecarType `json:"type"`
	// +optional
	Config *ReloadSidecarReference `json:"config,omitempty"`
}

type ReloadSidecarReference struct {
	// 对应 k8s-config 方式
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	Image string `json:"image,omitempty"`
	// +optional
	RestartPolicy corev1.ContainerRestartPolicy `json:"restartPolicy,omitempty"`
	// +optional
	Command []string `json:"command,omitempty"`

	// 对应 SidecarSet 方式
	// +optional
	SidecarSetRef *SidecarSetReference `json:"sidecarSetRef,omitempty"`

	// 对应 customer 方式
	// +optional
	ConfigMapRef *corev1.ObjectReference `json:"configMapRef,omitempty"`
}

type SidecarSetReference struct {
	Name          string `json:"name"`
	ContainerName string `json:"containerName"`
}

// EffectPolicyType 定义配置生效方式
type EffectPolicyType string

const (
	ReStartEffectPolicyType   EffectPolicyType = "ReStart"
	PostHookEffectPolicyType  EffectPolicyType = "PostHook"
	HotUpdateEffectPolicyType EffectPolicyType = "HotUpdate"
)

type EffectPolicy struct {
	Type EffectPolicyType `json:"type"`
	// +optional
	PostHook *PostHookConfig `json:"postHook,omitempty"`
}

type PostHookConfig struct {
	// +optional
	Exec *corev1.ExecAction `json:"exec,omitempty"`
	// +optional
	HTTPGet *corev1.HTTPGetAction `json:"httpGet,omitempty"`
	// +optional
	TCPSocket *corev1.TCPSocketAction `json:"tcpSocket,omitempty"`
}

type ConfigMapSetUpdateStrategy struct {
	// MatchLabelKeys is a set of pod label keys to select the pods over which rolling update logic will be applied.
	// +optional
	MatchLabelKeys []string `json:"matchLabelKeys,omitempty"`
	// +optional
	Partition *intstr.IntOrString `json:"partition,omitempty"` // 旧版本的比例
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"` // 最大同时不可用实例数
}

// ConfigMapSetStatus 定义观测状态
type ConfigMapSetStatus struct {
	// ObservedGeneration 用于防止控制器基于旧缓存做出更新决策（解决幻读）
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// CollisionCount 用于解决 Hash 碰撞导致的版本更新卡住问题
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`
	// CurrentRevision indicates the version of ConfigMapSet that most pods are using
	CurrentRevision string `json:"currentRevision,omitempty"`
	// CurrentCustomVersion indicates the custom version of ConfigMapSet that most pods are using
	CurrentCustomVersion string `json:"currentCustomVersion,omitempty"`
	// UpdateRevision indicates the version of ConfigMapSet that the controller is rolling out
	UpdateRevision string `json:"updateRevision,omitempty"`
	// UpdateCustomVersion indicates the custom version of ConfigMapSet that the controller is rolling out
	UpdateCustomVersion string `json:"updateCustomVersion,omitempty"`

	Replicas                int32        `json:"replicas"`                          // 关联Pod数量
	UpdatedReplicas         int32        `json:"updatedReplicas"`                   // 更新完成Pod数量
	ReadyReplicas           int32        `json:"readyReplicas"`                     // 健康Pod数量
	UpdatedReadyReplicas    int32        `json:"updatedReadyReplicas"`              // 健康且更新完成Pod数量
	ExpectedUpdatedReplicas int32        `json:"expectedUpdatedReplicas,omitempty"` // 期望更新的Pod数量
	LastContainersTimestamp *metav1.Time `json:"lastContainersTimestamp,omitempty"` // spec.containers最后一次更新时间, 用于冷启动决策
	LastContainersHash      string       `json:"lastContainersHash,omitempty"`      // 上一个cms版本spec.containers的hash值
	LastSpecTimestamp       *metav1.Time `json:"lastSpecTimestamp,omitempty"`       // spec最后一次更新时间
	LastSpecHash            string       `json:"lastSpecHash,omitempty"`            // 上一个cms版本spec的hash值
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cms,singular=configmapset,path=configmapsets,scope=Namespaced
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`,description="Number of replicas"
// +kubebuilder:printcolumn:name="UpdatedReplicas",type=integer,JSONPath=`.status.updatedReplicas`,description="Number of updated replicas"
// +kubebuilder:printcolumn:name="ReadyReplicas",type=integer,JSONPath=`.status.readyReplicas`,description="Number of ready replicas"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type ConfigMapSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ConfigMapSetSpec `json:"spec,omitempty"`
	// +optional
	Status ConfigMapSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type ConfigMapSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigMapSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigMapSet{}, &ConfigMapSetList{})
}
