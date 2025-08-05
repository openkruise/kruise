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
	// InjectedContainers 表示的是需要被注入配置的容器
	InjectedContainers []InjectedContainerSpec `json:"injectedContainers"`
	// RevisionHistoryLimit 用于描述 最多在RMC中维护多少个版本的数据
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
	// InjectUpdateOrder 为 true 表示采用仅注入策略, controller为需要重启的pod打上标签 configmapset.cms1/updateOrder=1
	// +optional
	InjectUpdateOrder bool `json:"injectUpdateOrder,omitempty"`
	// UpdateStrategy 指定更新策略, 详见结构体定义
	// +optional
	UpdateStrategy ConfigMapSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type InjectedContainerSpec struct {
	// +optional
	Name string `json:"name,omitempty"` // 需要挂载配置的容器名(静态注入)
	// +optional
	NameFrom  *ValueFromSource `json:"NameFrom,omitempty"` // 从pod中获取(动态注入), 如metadata.labels['cName']
	MountPath string           `json:"mountPath"`          // 容器内挂载路径
}

type ValueFromSource struct {
	FieldRef corev1.ObjectFieldSelector `json:"fieldRef"`
}

type Distribution struct {
	// +optional
	Revision string `json:"revision,omitempty"` // 版本hash
	// +optional
	CustomVersion string `json:"customVersion,omitempty"` // caster上的版本别名
	Reserved      int32  `json:"reserved"`                // 需要多少个该版本的实例
	// +optional
	Preferred bool `json:"preferred,omitempty"` // 当设置为true时, 如果所有distribution的数量加起来小于pod总数, 则其他的pod都使用该版本
}

type ConfigMapSetUpdateStrategy struct {
	// +optional
	Distributions []Distribution `json:"distributions,omitempty"`
	// +optional
	Partition *intstr.IntOrString `json:"partition,omitempty"` // 旧版本的比例, 多个旧版本的情况不做区分
	// +optional
	RestartInjectedContainers bool `json:"restartInjectedContainers,omitempty"` // 开启该选项后, 被注入最新配置的容器发生重启
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"` // 最大同时不可用实例数
}

// ConfigMapSetStatus 定义观测状态
type ConfigMapSetStatus struct {
	ObservedGeneration      int64        `json:"observedGeneration,omitempty"`
	CurrentRevision         string       `json:"currentRevision,omitempty"` // 当前版本hash, 即partition中最新的版本
	UpdateRevision          string       `json:"updateRevision,omitempty"`  // 目标版本hash, 即partition以外的版本
	MatchedPods             int32        `json:"matchedPods"`               // 根据标签匹配到的pod数量(不含terminating的)
	UpdatedPods             int32        `json:"updatedPods"`               // 已经最新的pod数量
	ReadyPods               int32        `json:"readyPods"`                 // 所有pod里面ready的数量
	UpdatedReadyPods        int32        `json:"updatedReadyPods"`          // 已经最新的pod里面ready的数量
	LastContainersTimestamp *metav1.Time `json:"lastContainersTimestamp"`   // spec.containers最后一次更新时间, 用于冷启动决策
	LastContainersHash      string       `json:"lastContainersHash"`        // 上一个cms版本spec.containers的hash值
	LastSpecTimestamp       *metav1.Time `json:"lastSpecTimestamp"`         // spec最后一次更新时间
	LastSpecHash            string       `json:"lastSpecHash"`              // 上一个cms版本spec的hash值
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cms,singular=configmapset,path=configmapsets,scope=Namespaced
// +kubebuilder:printcolumn:name="MatchedPods",type=integer,JSONPath=`.status.matchedPods`,description="Number of matched pods"
// +kubebuilder:printcolumn:name="UpdatedPods",type=integer,JSONPath=`.status.updatedPods`,description="Number of updated pods"
// +kubebuilder:printcolumn:name="ReadyPods",type=integer,JSONPath=`.status.readyPods`,description="Number of ready pods"
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
