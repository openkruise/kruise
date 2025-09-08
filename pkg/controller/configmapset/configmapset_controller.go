/*
Copyright 2016 The Kubernetes Authors.
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package configmapset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"math"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
	"strconv"
	"strings"
	"time"
)

func init() {
	flag.IntVar(&concurrentReconciles, "configmapset-workers", concurrentReconciles, "Max concurrent workers for ConfigMapSet controller.")
}

type RevisionKeys struct {
	CurrentRevisionKey          string
	CurrentRevisionTimeStampKey string
	TargetRevisionKey           string
	TargetRevisionTimeStampKey  string
	TargetCustomVersionKey      string
	CurrentCustomVersionKey     string
}

type SpecForHash struct {
	CustomVersion        string                               `json:"customVersion,omitempty"`
	Selector             *metav1.LabelSelector                `json:"selector"`
	Data                 map[string]string                    `json:"data"`
	InjectedContainers   []appsv1alpha1.InjectedContainerSpec `json:"injectedContainers"`
	RevisionHistoryLimit *int32                               `json:"revisionHistoryLimit,omitempty"`
	InjectUpdateOrder    bool                                 `json:"injectUpdateOrder,omitempty"`
}

type UpdateInfo struct {
	PodsToUpdate         []*corev1.Pod
	TargetRevisions      []string
	TargetCustomVersions []string
}

const (
	AnnotationPrefix                         = "apps.kruise.io/configmapset."
	AnnotationTargetRevisionSuffix           = ".revision"
	AnnotationCurrentRevisionSuffix          = ".currentRevision"
	AnnotationTargetRevisionTimeStampSuffix  = ".revisionTimestamp"
	AnnotationCurrentRevisionTimeStampSuffix = ".currentRevisionTimestamp"
	AnnotationTargetCustomVersionSuffix      = ".customVersion"
	AnnotationCurrentCustomVersionSuffix     = ".currentCustomVersion"
	ConfigMapFinalizerName                   = "finalizer.configmapset.bilibili.co"
)

var (
	concurrentReconciles                      = 3
	controllerKind                            = appsv1alpha1.SchemeGroupVersion.WithKind("ConfigMapSet")
	resourceVersionExpectations               = expectations.NewResourceVersionExpectation()
	retryDuration               time.Duration = 15
)

// RevisionEntry 定义存储 revisions 的数据结构
type RevisionEntry struct {
	Hash          string            `json:"hash"`
	CustomVersion string            `json:"customVersion"`
	TimeStamp     *metav1.Time      `json:"timeStamp"`
	Data          map[string]string `json:"data"`
}

// Add creates a new ConfigMapSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("configmapset-controller")
	return &ReconcileConfigMapSet{
		Client:   utilclient.NewClientFromManager(mgr, "configmapset-controller"),
		scheme:   mgr.GetScheme(),
		recorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("configmapset-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter[reconcile.Request]()})
	if err != nil {
		return err
	}

	// Watch for changes to ConfigMapSet
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.ConfigMapSet{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.ConfigMapSet]{}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}, &podEventHandler{Reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileConfigMapSet{}

// ReconcileConfigMapSet reconciles a ConfigMapSet object
type ReconcileConfigMapSet struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=configmapsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=configmapsets/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a ConfigMapSet object and makes changes based on the state read
// and what is in the ConfigMapSet.Spec
func (r *ReconcileConfigMapSet) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ConfigMapSet instance
	cms := &appsv1alpha1.ConfigMapSet{}
	err := r.Get(ctx, request.NamespacedName, cms)

	// 打印日志信息
	klog.Warningf("start to reconcile ConfigMapSet %s", request.String())

	// 处理删除逻辑
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			// cms 被删除, 不影响存量的pod
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		klog.Errorf("failed to get ConfigMapSet %s: %v", request.NamespacedName, err)
		return reconcile.Result{}, err
	}

	// If resourceVersion expectations have not satisfied yet, just skip this reconcile
	resourceVersionExpectations.Observe(cms)
	if isSatisfied, unsatisfiedDuration := resourceVersionExpectations.IsSatisfied(cms); !isSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			klog.Warningf("Expectation unsatisfied overtime for %v, timeout=%v", request.String(), unsatisfiedDuration)
			return reconcile.Result{}, nil
		}
		klog.V(4).Infof("Not satisfied resourceVersion for %v", request.String())
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	// 如果正在被删除
	if cms.DeletionTimestamp != nil && !cms.DeletionTimestamp.IsZero() {
		if containsString(cms.Finalizers, ConfigMapFinalizerName) {
			if err := r.cleanupConfigMap(ctx, cms); err != nil {
				return reconcile.Result{}, fmt.Errorf("cleanupConfigMap failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
			}

			// 移除 Finalizer，允许 CMS 资源被真正删除
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				latest := &appsv1alpha1.ConfigMapSet{}
				if err := r.Get(ctx, types.NamespacedName{Name: cms.Name, Namespace: cms.Namespace}, latest); err != nil {
					return err
				}
				latest.Finalizers = removeString(latest.Finalizers, ConfigMapFinalizerName)
				return r.Update(ctx, latest)
			})
			if err != nil {
				klog.Errorf("RetryOnConflict failed to remove finalizer for %s/%s: %v", cms.Namespace, cms.Name, err)
				return reconcile.Result{}, fmt.Errorf("failed to remove finalizer for %s/%s: %w", cms.Namespace, cms.Name, err)
			}
		}
		klog.Infof("ConfigMapSet %s is being deleted", cms.Name)
		return reconcile.Result{}, nil
	}

	// 添加 Finalizer（如果还没有）
	if !containsString(cms.Finalizers, ConfigMapFinalizerName) {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest := &appsv1alpha1.ConfigMapSet{}
			if err := r.Get(ctx, types.NamespacedName{Name: cms.Name, Namespace: cms.Namespace}, latest); err != nil {
				return err
			}
			if !containsString(latest.Finalizers, ConfigMapFinalizerName) {
				latest.Finalizers = append(latest.Finalizers, ConfigMapFinalizerName)
				return r.Update(ctx, latest)
			}
			return nil
		})
		if err != nil {
			klog.Errorf("RetryOnConflict failed to add finalizer for %s/%s: %v", cms.Namespace, cms.Name, err)
			return reconcile.Result{}, fmt.Errorf("failed to add finalizer for %s/%s: %w", cms.Namespace, cms.Name, err)
		}
	}

	hashChangedFlag := false

	// 冷启动状态处理
	// 对spec.containers做hash
	// 如果和status.lastContainersHash不一致, 说明更新了
	newContainersHash, err := CalculateHash(cms.Spec.InjectedContainers)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to compute spec.containers hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}
	if newContainersHash != cms.Status.LastContainersHash { // spec.containers发生了变化
		hashChangedFlag = true
		cms.Status.LastContainersHash = newContainersHash
		cms.Status.LastContainersTimestamp = &metav1.Time{Time: metav1.Now().Time}
	}

	// 记录spec的修改
	newSpecHash, err := CalculateHash(cms.Spec)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to compute spec hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}
	if newSpecHash != cms.Status.LastSpecHash { // spec发生了变化
		klog.Warningf("cms spec changed, lastSpecHash: %s, newSpecHash: %s", cms.Status.LastSpecHash, newSpecHash)
		hashChangedFlag = true
		cms.Status.LastSpecHash = newSpecHash
		cms.Status.LastSpecTimestamp = &metav1.Time{Time: metav1.Now().Time}
	}

	// 版本管理
	revisionEntry, err := r.manageRevisions(ctx, cms)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("manageRevisions failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	klog.Infof("updated revision for cms %s/%s, revision is %v", cms.Namespace, cms.Name, revisionEntry)

	pods, err := GetMatchedPods(ctx, r.Client, cms)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("GetMatchedPods failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	for _, pod := range pods {
		resourceVersionExpectations.Observe(pod)
		// 只要有一个pod不是最新状态, 就把整个流程都跳过
		if isSatisfied, unsatisfiedDuration := resourceVersionExpectations.IsSatisfied(pod); !isSatisfied {
			if unsatisfiedDuration >= expectations.ExpectationTimeout {
				klog.Warningf("Expectation unsatisfied overtime for %v, wait for pod %v updating, timeout=%v", request.String(), pod.Name, unsatisfiedDuration)
				return reconcile.Result{}, nil
			}
			klog.V(4).Infof("Not satisfied resourceVersion for %v, wait for pod %v updating", request.String(), pod.Name)
			return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
		}
	}

	// Pod同步
	err, shouldRetry := r.syncPods(ctx, cms, revisionEntry, pods)
	if err != nil {
		if shouldRetry {
			klog.Warningf("syncPods failed for cms %s/%s: %v, retry in %d seconds", cms.Namespace, cms.Name, err, retryDuration)
			return reconcile.Result{RequeueAfter: time.Second * retryDuration}, nil
		}
		return reconcile.Result{}, fmt.Errorf("syncPods failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}
	// err == nil 时也可能会需要重试, 在有CRR存在时 / pod未重启需要回调版本时
	if shouldRetry {
		klog.Warningf("pod sync not finished for cms %s/%s, retry in %d seconds", cms.Namespace, cms.Name, retryDuration)
		return reconcile.Result{RequeueAfter: time.Second * retryDuration}, nil
	}

	// 更新状态
	err = r.updateStatus(ctx, request, cms, hashChangedFlag)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("updateStatus failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}
	resourceVersionExpectations.Expect(cms)
	return reconcile.Result{}, nil
}

func (r *ReconcileConfigMapSet) cleanupConfigMap(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	cmName := fmt.Sprintf("%s-hub", cms.Name)
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cms.Namespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil // ConfigMap 已经被删除
		}
		return err
	}

	// 删除 ConfigMap
	return r.Delete(ctx, cm)
}

// CalculateHash 计算对象的哈希值
func CalculateHash(v interface{}) (string, error) {
	if v == nil {
		return "", fmt.Errorf("object is nil")
	}
	// 1. 将 spec 序列化为 JSON
	specBytes, err := json.Marshal(v)
	if err != nil {
		// 这里的 panic 主要是为了确保 JSON 序列化不会失败，正常情况下不会触发
		return "", fmt.Errorf("failed to marshal spec: %v", err)
	}

	// 2. 计算 SHA256 哈希
	hash := sha256.Sum256(specBytes)

	// 3. 返回十六进制编码的哈希值
	return hex.EncodeToString(hash[:8]), nil // 取前 8 字节作为短哈希
}

// 版本管理逻辑
func (r *ReconcileConfigMapSet) manageRevisions(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) ([]RevisionEntry, error) {
	klog.Infof("Manage revisions for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	// 计算当前 Spec 的 Hash
	hash, err := CalculateHash(cms.Spec.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash: %v", err)
	}

	// ConfigMap 命名：cms.Name + "-hub"
	cmName := fmt.Sprintf("%s-hub", cms.Name)
	cmNamespace := cms.Namespace

	var revisions []RevisionEntry

	// 定义更新逻辑
	updateFunc := func() error {
		// 重新获取最新的 ConfigMap，避免并发冲突
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
			if errors.IsNotFound(err) {
				// 如果 ConfigMap 不存在，则创建
				newCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmName,
						Namespace: cmNamespace,
					},
					Data: make(map[string]string),
				}
				return r.Create(ctx, newCM)
			}
			return fmt.Errorf("failed to get ConfigMap: %v", err)
		}

		// 解析现有 ConfigMap 的 revisions
		if revData, exists := cm.Data["revisions"]; exists {
			if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
				klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
				return fmt.Errorf("failed to unmarshal revisions from ConfigMap %s: %v", cmName, err)
			}
		}

		// 检查 Hash 是否已存在
		for i, rev := range revisions {
			if rev.Hash == hash {
				klog.Warningf("Revision %s already exists in ConfigMap %s, skipping update", hash, cmName)
				// hash允许重复
				// 但是customVersion不可以同时也重复
				if rev.CustomVersion == cms.Spec.CustomVersion {
					// 回滚的场景
					// 修改re, 但是不更新configmap
					revisions[i].TimeStamp = &metav1.Time{Time: time.Now()}
					return nil
				}
			}
		}

		// 插入新的 revision
		revisions = append(revisions, RevisionEntry{
			Hash:          hash,
			TimeStamp:     &metav1.Time{Time: time.Now()},
			Data:          cms.Spec.Data,
			CustomVersion: cms.Spec.CustomVersion, // 这里可能为空
		})

		klog.Infof("Updated revisions for ConfigMapSet %s/%s: %v", cms.Namespace, cms.Name, revisions)

		// 更新 ConfigMap Data
		revBytes, err := json.Marshal(revisions)
		if err != nil {
			return fmt.Errorf("failed to marshal revisions: %v", err)
		}
		cm = cm.DeepCopy() // 避免修改缓存对象
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data["revisions"] = string(revBytes)

		// 更新 ConfigMap
		return r.Update(ctx, cm)
	}

	// 使用 `retry.RetryOnConflict` 处理更新冲突
	err = retry.RetryOnConflict(retry.DefaultRetry, updateFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to update ConfigMap %s: %v", cmName, err)
	}

	return revisions, nil
}

// Pod同步逻辑
func (r *ReconcileConfigMapSet) SyncPods(ctx context.Context, client client.Client, cms *appsv1alpha1.ConfigMapSet, re []RevisionEntry, pods []*corev1.Pod) (error, bool) {
	klog.Infof("Syncing pods for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	if len(re) == 0 {
		return fmt.Errorf("revision list is empty for cms %s", cms.Name), false
	}

	revisionKeys := &RevisionKeys{
		CurrentRevisionKey:          fmt.Sprintf("%s%s%s", AnnotationPrefix, cms.Name, AnnotationCurrentRevisionSuffix),
		CurrentRevisionTimeStampKey: fmt.Sprintf("%s%s%s", AnnotationPrefix, cms.Name, AnnotationCurrentRevisionTimeStampSuffix),
		TargetRevisionKey:           fmt.Sprintf("%s%s%s", AnnotationPrefix, cms.Name, AnnotationTargetRevisionSuffix),
		TargetRevisionTimeStampKey:  fmt.Sprintf("%s%s%s", AnnotationPrefix, cms.Name, AnnotationTargetRevisionTimeStampSuffix),
		TargetCustomVersionKey:      fmt.Sprintf("%s%s%s", AnnotationPrefix, cms.Name, AnnotationTargetCustomVersionSuffix),
		CurrentCustomVersionKey:     fmt.Sprintf("%s%s%s", AnnotationPrefix, cms.Name, AnnotationCurrentCustomVersionSuffix),
	}
	// 根据更新策略选择要更新的Pod
	updateStrategy := cms.Spec.UpdateStrategy

	// 定义了partition的情形
	if updateStrategy.Partition != nil {
		distributions, err := TransformPartition2Distribution(cms, re, pods)
		if err != nil {
			klog.Errorf("failed to transform partition to distribution: %v", err)
			return fmt.Errorf("failed to transform partition to distribution: %v", err), false
		}
		return r.UpdateByDistribution(ctx, client, cms, distributions, re, pods, revisionKeys)
	}

	// 使用distribution的情形
	if len(updateStrategy.Distributions) > 0 {
		return r.UpdateByDistribution(ctx, client, cms, cms.Spec.UpdateStrategy.Distributions, re, pods, revisionKeys)
	}

	// 都没有定义的情况理论上不会出现(webhook会拦截)
	return fmt.Errorf("updateStrategy is not defined"), false
}

func TransformPartition2Distribution(cms *appsv1alpha1.ConfigMapSet, re []RevisionEntry, pods []*corev1.Pod) ([]appsv1alpha1.Distribution, error) {
	// 总副本数 = 传入的pod数
	replicas := len(pods)
	var updateCustomVersion, currentCustomVersion string
	// 解析 partition
	partition := cms.Spec.UpdateStrategy.Partition

	if len(re) == 0 {
		return nil, fmt.Errorf("revision list is empty for cms %s/%s", cms.Namespace, cms.Name)
	}

	// 由于syncPods在updateStatus之前, 这里需要计算当前最新的updateRevision
	updateRevision, err := CalculateHash(cms.Spec.Data) // 计算当前版本
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	// 特殊情况1: 只有一个版本的时候, 一定是全量
	// 特殊情况2: 回滚的场景, 而上一次升级并没有升级完, 就可能会变成 updateRevision == currentRevision 的情形
	if len(re) == 1 || updateRevision == cms.Status.CurrentRevision {
		distributions := make([]appsv1alpha1.Distribution, 1)
		distributions[0] = appsv1alpha1.Distribution{
			Revision:      updateRevision,
			Reserved:      int32(replicas),
			Preferred:     true,
			CustomVersion: re[0].CustomVersion,
		}
		klog.Infof("distributions after transform: %v", distributions)
		return distributions, nil
	}

	for i := 0; i < len(re); i++ {
		if re[i].Hash == updateRevision {
			updateCustomVersion = re[i].CustomVersion
		} else if re[i].Hash == cms.Status.CurrentRevision {
			currentCustomVersion = re[i].CustomVersion
		}
	}
	newReplicas := replicas // 新实例数
	if partition != nil {
		partitionStr := partition.String()
		if strings.HasSuffix(partitionStr, "%") {
			// 处理百分比 partition
			percentStr := strings.TrimSuffix(partitionStr, "%")
			percent, err := strconv.Atoi(percentStr)
			if err != nil || percent < 0 || percent > 100 {
				return nil, fmt.Errorf("invalid partition percentage: %s", partitionStr)
			}
			newReplicas = replicas - int(math.Ceil(float64(percent/100)*float64(replicas)))
		} else if count, err := strconv.Atoi(partitionStr); err == nil && count >= 0 {
			// 处理整数 partition
			newReplicas = replicas - count
			if newReplicas < 0 { // 不能出现负数
				newReplicas = 0
			}
		} else {
			return nil, fmt.Errorf("invalid partition number: %s", partitionStr)
		}
	}
	// 构造distribution
	distributions := make([]appsv1alpha1.Distribution, 2)
	distributions[0] = appsv1alpha1.Distribution{
		Revision:      updateRevision,
		Reserved:      int32(newReplicas),
		Preferred:     true,
		CustomVersion: updateCustomVersion,
	}
	distributions[1] = appsv1alpha1.Distribution{
		Revision:      cms.Status.CurrentRevision,
		Reserved:      int32(replicas - newReplicas),
		CustomVersion: currentCustomVersion,
	}

	klog.Infof("distributions after transform: %v", distributions)
	return distributions, nil
}

func FetchUpdateInfoByDistribution(distributions []appsv1alpha1.Distribution, re []RevisionEntry, pods []*corev1.Pod, revisionKeys *RevisionKeys) (*UpdateInfo, error) {
	//distributions := cms.Spec.UpdateStrategy.Distributions
	distributionsCopy := make([]appsv1alpha1.Distribution, len(distributions))
	copy(distributionsCopy, distributions)
	// 构建 revision 索引，提高查询性能
	rmcIndexMap := make(map[string]int, len(re))
	customVersionToRevision := make(map[string]string)
	for i, rev := range re {
		rmcIndexMap[rev.Hash] = i
		if rev.CustomVersion != "" {
			customVersionToRevision[rev.CustomVersion] = rev.Hash
		}
	}
	klog.Infof("customVersionToRevision: %v", customVersionToRevision)

	// 为distributionsCopy中使用customVersion的添加对应的revision
	for i := range distributionsCopy {
		if distributionsCopy[i].Revision == "" {
			distributionsCopy[i].Revision = customVersionToRevision[distributionsCopy[i].CustomVersion]
		}
		// 如果仍然为空, 说明版本已经过期了
		// 这种情况, 拒绝更新
		if distributionsCopy[i].Revision == "" {
			return nil, fmt.Errorf("custom version %s not found", distributionsCopy[i].CustomVersion)
		}
		// 如果re中找不到这个版本, 也拒绝更新
		if _, exist := rmcIndexMap[distributionsCopy[i].Revision]; !exist {
			return nil, fmt.Errorf("revision %s not found", distributionsCopy[i].Revision)
		}
	}

	// 统计当前每个版本的 pod 分布
	revisionToPods := make(map[string][]*corev1.Pod)
	revisionCount := make(map[string]int32)

	for _, pod := range pods {
		rev := pod.Annotations[revisionKeys.CurrentRevisionKey]
		revisionToPods[rev] = append(revisionToPods[rev], pod)
		revisionCount[rev]++
	}

	// 需要对revisionToPods中的pod排序, 保证每次的顺序一致
	// 为了防止重启多个实例
	for _, peerPods := range revisionToPods {
		sort.Slice(peerPods, func(i, j int) bool {
			return peerPods[i].Name < peerPods[j].Name
		})
	}

	klog.Infof("revision count: %v", revisionCount)
	klog.Infof("revision pods: ")
	for rev, pods := range revisionToPods {
		klog.Infof("revision: %s", rev)
		for _, p := range pods {
			klog.Infof("%s/%s/%s\t", p.Namespace, p.Name, p.UID)
		}
	}

	// 计算 preferred 补充数量
	totalPods := int32(len(pods))
	reservedCount := int32(0)
	var preferredRevision string
	// 收集待更新 pod（多余版本 -> 目标版本）
	var podsToUpdate []*corev1.Pod
	targetRevisions := make([]string, 0, totalPods)
	targetCustomVersions := make([]string, 0, totalPods)

	for _, d := range distributionsCopy {
		reservedCount += d.Reserved
		if d.Preferred {
			preferredRevision = d.Revision
		}
	}

	klog.Infof("reservedCount: %d", reservedCount)
	klog.Infof("distributionsCopy before preferred: %v", distributionsCopy)

	// 如果总实例数不够分
	if reservedCount > totalPods {
		podsToUpdate, targetRevisions, targetCustomVersions = fetchPodsLessThanReserved(podsToUpdate, totalPods, distributionsCopy, rmcIndexMap, targetRevisions, targetCustomVersions, revisionToPods)
	} else {
		podsToUpdate, targetRevisions, targetCustomVersions = fetchPodsMoreThanReserved(totalPods, reservedCount, distributionsCopy, preferredRevision, revisionCount, podsToUpdate, revisionToPods, revisionKeys, targetRevisions, targetCustomVersions)
	} // end of else
	return &UpdateInfo{
		PodsToUpdate:         podsToUpdate,
		TargetRevisions:      targetRevisions,
		TargetCustomVersions: targetCustomVersions,
	}, nil
}

func fetchPodsMoreThanReserved(totalPods int32, reservedCount int32, distributionsCopy []appsv1alpha1.Distribution, preferredRevision string, revisionCount map[string]int32, podsToUpdate []*v1.Pod, revisionToPods map[string][]*v1.Pod, revisionKeys *RevisionKeys, targetRevisions []string, targetCustomVersions []string) ([]*v1.Pod, []string, []string) {
	extra := totalPods - reservedCount
	for i := range distributionsCopy {
		if distributionsCopy[i].Revision == preferredRevision {
			distributionsCopy[i].Reserved += extra // 手动加上这一部分
			break
		}
	}

	klog.Infof("distributionsCopy after preferred: %v", distributionsCopy)

	for _, d := range distributionsCopy {
		actual := revisionCount[d.Revision] // 版本对应的现有pod数
		klog.Infof("revision: %s, actual: %d, desired: %d", d.Revision, actual, d.Reserved)
		if actual > d.Reserved { // 如果现有的pod数 > 期望值
			// 把多出来的部分都放到podsToUpdate里
			podsToUpdate = append(podsToUpdate, revisionToPods[d.Revision][:actual-d.Reserved]...)
			// 剩下的pod, 可以保留当前的currentRevision
			// 如果targetRevision是别的版本, 那么也需要加进来
			for _, p := range revisionToPods[d.Revision][actual-d.Reserved:] {
				if p.Annotations[revisionKeys.TargetRevisionKey] != d.Revision {
					podsToUpdate = append(podsToUpdate, p)
					targetRevisions = append(targetRevisions, d.Revision)
					targetCustomVersions = append(targetCustomVersions, d.CustomVersion)
				}
			}
		} else {
			// 需要更新一些pod到该版本
			for i := int32(0); i < d.Reserved-actual; i++ {
				targetRevisions = append(targetRevisions, d.Revision)
				targetCustomVersions = append(targetCustomVersions, d.CustomVersion)
			}
			// 如果现有pod的targetRevision是别的版本, 那么也需要加进来
			for _, p := range revisionToPods[d.Revision] {
				if p.Annotations[revisionKeys.TargetRevisionKey] != d.Revision {
					klog.Infof("adding pod %s/%s to update, since its target revision %s does not match current revision %s",
						p.Namespace, p.Name, p.Annotations[revisionKeys.TargetRevisionKey], d.Revision)
					podsToUpdate = append(podsToUpdate, p)
					targetRevisions = append(targetRevisions, d.Revision)
					targetCustomVersions = append(targetCustomVersions, d.CustomVersion)
				}
			}
		}
	}
	// 还有一些pod的revision没有被记录在distributionsCopy里面, 这些pod全都应该被更新
	for rev, pods := range revisionToPods {
		if notExistInDistribution(rev, distributionsCopy) {
			podsToUpdate = append(podsToUpdate, pods...)
		}
	}
	// 需要对podsToUpdate排序
	// 让currentRevision与targetRevision一致的放在一起
	totalPodsToUpdate := len(podsToUpdate)
	revToPods := make(map[string][]*corev1.Pod)
	for _, p := range podsToUpdate {
		rev := p.Annotations[revisionKeys.CurrentRevisionKey]
		revToPods[rev] = append(revToPods[rev], p)
	}
	podsToUpdate = make([]*corev1.Pod, totalPodsToUpdate)
	for i, rev := range targetRevisions {
		if i >= totalPodsToUpdate { // 后面的版本不需要处理
			break
		}
		if podList, exist := revToPods[rev]; exist && len(podList) > 0 {
			podsToUpdate[i] = podList[0]
			// 删除这个pod
			revToPods[rev] = podList[1:]
		} else {
			podsToUpdate[i] = nil
		}
	}
	// 填充未匹配的空位
	left := make([]*corev1.Pod, 0)
	for _, podList := range revToPods {
		left = append(left, podList...)
	}
	leftIdx := 0
	for i := range podsToUpdate {
		if podsToUpdate[i] == nil && leftIdx < len(left) {
			podsToUpdate[i] = left[leftIdx]
			leftIdx++
		}
	}
	return podsToUpdate, targetRevisions, targetCustomVersions
}

func fetchPodsLessThanReserved(podsToUpdate []*v1.Pod, totalPods int32, distributionsCopy []appsv1alpha1.Distribution, rmcIndexMap map[string]int, targetRevisions []string, targetCustomVersions []string, revisionToPods map[string][]*v1.Pod) ([]*v1.Pod, []string, []string) {
	// 这种情况, 我们更新所有pod
	podsToUpdate = make([]*corev1.Pod, totalPods)
	// 对distributionsCopy排序
	// preferred 优先更新
	// 版本更新的 优先更新
	sort.Slice(distributionsCopy, func(i, j int) bool {
		// Preferred 优先
		if distributionsCopy[i].Preferred != distributionsCopy[j].Preferred {
			return distributionsCopy[i].Preferred
		}
		// 不需要考虑找不到版本的问题, 前面已经处理过了
		return rmcIndexMap[distributionsCopy[i].Revision] > rmcIndexMap[distributionsCopy[j].Revision]
	})

	for _, d := range distributionsCopy { // 不管有多少, 全加上去, 多出来的部分不处理即可
		for i := int32(0); i < d.Reserved; i++ {
			targetRevisions = append(targetRevisions, d.Revision)
			targetCustomVersions = append(targetCustomVersions, d.CustomVersion)
		}
	}

	for i, rev := range targetRevisions {
		if int32(i) >= totalPods { // 后面的不处理
			break
		}
		// 先找自己版本的pod, 尽量减少更新
		if pods, exist := revisionToPods[rev]; exist && len(pods) > 0 {
			podsToUpdate[i] = pods[0]
			// 从map中移除pods[0]
			if len(pods) > 1 {
				revisionToPods[rev] = revisionToPods[rev][1:]
			} else {
				delete(revisionToPods, rev) // 防止后续访问空 slice
			}
		} else {
			// 如果没有自己版本的pod, 先空着
			podsToUpdate[i] = nil
		}
	}
	// 打印当前的结果
	klog.Infof("podsToUpdate after processing: ")
	for i, p := range podsToUpdate {
		if p == nil {
			klog.Infof("%d: nil\t", i)
			continue
		}
		klog.Infof("%d: %s/%s/%s\t", i, p.Namespace, p.Name, p.UID)
	}

	// 结束以后, 把revisionToPods里剩下的pod填充进去
	var leftPods []*corev1.Pod
	for _, pods := range revisionToPods {
		if len(pods) > 0 {
			leftPods = append(leftPods, pods...)
		}
	}
	klog.Infof("leftPods: ")
	for i, p := range leftPods {
		klog.Infof("%d: %s/%s/%s\t", i, p.Namespace, p.Name, p.UID)
	}
	for i := range podsToUpdate {
		if len(leftPods) == 0 {
			break
		}
		if podsToUpdate[i] == nil {
			podsToUpdate[i] = leftPods[0]
			leftPods = leftPods[1:]
		}
	}
	return podsToUpdate, targetRevisions, targetCustomVersions
}

func (r *ReconcileConfigMapSet) UpdateByDistribution(ctx context.Context, c client.Client, cms *appsv1alpha1.ConfigMapSet, distributions []appsv1alpha1.Distribution, re []RevisionEntry, pods []*corev1.Pod, revisionKeys *RevisionKeys) (error, bool) {

	updateInfo, err := FetchUpdateInfoByDistribution(distributions, re, pods, revisionKeys)
	if err != nil {
		klog.Errorf("failed to fetch update info: %v", err)
		return err, false
	}
	// 打印targetRevisions信息
	klog.Infof("target revisions: %v", updateInfo.TargetRevisions)
	klog.Infof("target custom versions: %v", updateInfo.TargetCustomVersions)

	tsMap := make(map[string]*metav1.Time)
	// 记录每个revision的TS
	for _, v := range re {
		tsMap[v.Hash] = v.TimeStamp
	}
	// 执行更新 pod 的 annotation 和 label
	var errs []error
	if len(updateInfo.PodsToUpdate) > len(updateInfo.TargetRevisions) {
		// 异常情况
		// 打印日志并退出
		return fmt.Errorf("podsToUpdate(%d) > targetRevisions(%d)", len(updateInfo.PodsToUpdate), len(updateInfo.TargetRevisions)), false
	}

	klog.Infof("pods to update: ")
	for _, p := range updateInfo.PodsToUpdate {
		klog.Infof("%s/%s\t", p.Namespace, p.Name)
	}

	// 在一轮更新前就计算好最多能更新几个pod
	var notReadyNum, crrNum int
	// pods 不包含terminating的, 但是包含新启动的
	// update 要求 : maxUnavailable >= notReadyNum + crrNum
	for _, p := range pods {
		klog.Infof("[UpdateByDistribution] current pod: %s/%s", p.Namespace, p.Name)
		if IsPodReady(p) {
			// ready的得看有没有潜在的crr, 如果有, 也得算进去
			klog.Infof("[UpdateByDistribution] Pod %s/%s is ready", p.Namespace, p.Name)
			crrList := &appsv1alpha1.ContainerRecreateRequestList{}
			err := c.List(context.TODO(), crrList, client.InNamespace(p.Namespace), client.MatchingLabels{appsv1alpha1.ContainerRecreateRequestPodUIDKey: string(p.UID)})
			if err != nil {
				klog.Errorf("Failed to get CRR List for Pod %s/%s: %v", p.Namespace, p.Name, err)
				return fmt.Errorf("failed to get CRR List for Pod %s/%s: %v", p.Namespace, p.Name, err), false
			}
			if len(crrList.Items) > 0 {
				// 说明存在Pending的CRR, 需要计入容忍度里
				klog.Infof("Found CRR List for Pod %s/%s, crr: %v", p.Namespace, p.Name, crrList.Items)
				crrNum++
			}
		} else {
			// 不ready的肯定要算进去
			klog.Infof("[UpdateByDistribution] Pod %s/%s is not ready", p.Namespace, p.Name)
			notReadyNum++
		}
	}
	klog.Infof("notReadyNum: %d, crrNum: %d", notReadyNum, crrNum)
	matchedPods := len(pods)
	maxUpdateNum := int32(0)
	maxUnavailable := cms.Spec.UpdateStrategy.MaxUnavailable
	if maxUnavailable.Type == intstr.Int {
		maxUpdateNum = maxUnavailable.IntVal - int32(crrNum) - int32(notReadyNum)
	} else if maxUnavailable.Type == intstr.String {
		str := maxUnavailable.String()
		// 解析百分数
		if strings.HasSuffix(str, "%") {
			percentStr := strings.TrimSuffix(str, "%")
			percent, err := strconv.ParseFloat(percentStr, 64)
			if err != nil || percent < 0 || percent > 100 {
				klog.Errorf("invalid partition percentage: %s", str)
				return fmt.Errorf("invalid partition percentage: %s", str), false
			}
			// 向下取整, 保证不超并发度, 但不能 < 1
			maxUpdateNum = int32(math.Max(1, math.Floor(float64(matchedPods)*percent/100)))
			maxUpdateNum = maxUpdateNum - int32(crrNum) - int32(notReadyNum) // 扣除已有的crr
		} else {
			klog.Errorf("invalid partition percentage: %s", str)
			return fmt.Errorf("invalid partition percentage: %s", str), false
		}
	}

	for i, pod := range updateInfo.PodsToUpdate {
		klog.Infof("processing pod %s/%s, idx = %d, maxUpdateNum = %d", pod.Namespace, pod.Name, i, maxUpdateNum)
		newRevision := updateInfo.TargetRevisions[i] // 不会发生越界
		newCustomVersion := updateInfo.TargetCustomVersions[i]
		asyncUpdateLabel := "configmapset." + cms.Name + "/updateOrder"

		if maxUpdateNum <= 0 {
			// 前面的CRR已提交, 这里直接返回, 等待重试即可
			//klog.Warningf("max update num already reached, retry in next reconcile...")
			klog.Warningf("cannot satisfy maxUnavailable condition, update pod %s/%s to revision %s will retry...", pod.Namespace, pod.Name, newRevision)
			return fmt.Errorf("cannot satisfy maxUnavailable condition, update pod %s/%s to revision %s will retry", pod.Namespace, pod.Name, newRevision), true
		}

		p := &corev1.Pod{}
		if err := c.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, p); err != nil {
			return err, false
		}

		if p.Annotations == nil {
			p.Annotations = make(map[string]string)
		}

		// 已经使用目标版本hash的pod, 不需要进行版本变更
		if val, exists := p.Annotations[revisionKeys.TargetRevisionKey]; exists && val == newRevision {
			err, shouldRetry := HandleUpdatedPod(ctx, c, revisionKeys, tsMap, cms, asyncUpdateLabel, p)
			if err == nil && !shouldRetry {
				// 说明这个pod已经更新完成
				continue
			}
			return err, shouldRetry
		}

		e, shouldRetry, needRestart := r.HandlePodUpdate(ctx, c, pod, revisionKeys, newRevision, tsMap, cms, asyncUpdateLabel, newCustomVersion)

		if e != nil {
			// 无法创建CRR被拒绝, 这里直接返回, 重试
			if shouldRetry {
				klog.Warningf("cannot create CRR, update pod %s/%s to revision %s will retry", pod.Namespace, pod.Name, newRevision)
				return fmt.Errorf("cannot create CRR, update pod %s/%s to revision %s will retry", pod.Namespace, pod.Name, newRevision), true
			}
			errs = append(errs, err)
			klog.Errorf("Failed to update pod %s/%s to revision %s: %v", pod.Namespace, pod.Name, newRevision, err)
		}
		if needRestart {
			// 有pod需要重启, 那么本轮reconcile的最大更新数-1
			maxUpdateNum--
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("updateByDistribution: %d pod updates failed: %v", len(errs), errs), false
	}
	return nil, false
}

// 只更新, 不提交
func updatePod(p *corev1.Pod, revisionKeys *RevisionKeys, tsMap map[string]*metav1.Time, cms *appsv1alpha1.ConfigMapSet, targetRevision, customVersion, targetRevisionTimeStamp, asyncUpdateLabel string) error {
	// 打印pod的信息
	klog.Infof("Update pod %s/%s to revision %s", p.Namespace, p.Name, targetRevision)
	p.Annotations[revisionKeys.TargetRevisionKey] = targetRevision
	if _, ok := tsMap[targetRevision]; ok {
		// 更新annotation的时候使用cms的时间戳, 表示当前pod是在这一时刻被更新的
		p.Annotations[revisionKeys.TargetRevisionTimeStampKey] = targetRevisionTimeStamp
	} else {
		// 可能是比较古老的版本
		// 不设置targetRevisionTSKey
		// do nothing
	}
	p.Annotations[revisionKeys.TargetCustomVersionKey] = customVersion
	// 如果currentRevision 和 targetRevision相同, 不需要更新主容器
	// 可能发生在回滚的情形
	// current尚未更新, 也就是说重启还没完成/没有重启
	// 没有重启, 被回滚, 应该不处理
	// 重启过程中发生回滚, 应该再重启
	if p.Annotations[revisionKeys.CurrentRevisionKey] == targetRevision {
		// 不做标记
	} else {
		if cms.Spec.InjectUpdateOrder {
			if p.Labels == nil {
				p.Labels = make(map[string]string)
			}
			p.Labels[asyncUpdateLabel] = "1"
		}
	}
	return nil
}

func submitPod(ctx context.Context, c client.Client, pod *v1.Pod) (*v1.Pod, error) {
	klog.Infof("pod resourceVersion before submit: %s", pod.ResourceVersion)
	defer klog.Infof("pod resourceVersion after submit: %s", pod.ResourceVersion)
	return pod, c.Update(ctx, pod)
}

// 只更新, 不提交
func rollbackPod(ctx context.Context, c client.Client, newPod *v1.Pod, cms *appsv1alpha1.ConfigMapSet, revisionKeys *RevisionKeys, tsMap map[string]*metav1.Time, newRevision, asyncUpdateLabel string) error {
	oldPod := &corev1.Pod{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(newPod), oldPod); err != nil {
		return err
	}
	klog.Infof("rollback pod %s/%s with resourceVersion=%s, targetRevision=%s", oldPod.Namespace, oldPod.Name, oldPod.ResourceVersion, oldPod.Annotations[revisionKeys.TargetRevisionKey])
	if newPod.Annotations == nil {
		newPod.Annotations = make(map[string]string)
	}
	if oldVal, ok := oldPod.Annotations[revisionKeys.TargetRevisionKey]; ok {
		newPod.Annotations[revisionKeys.TargetRevisionKey] = oldVal
	}
	if oldVal, ok := oldPod.Annotations[revisionKeys.TargetCustomVersionKey]; ok {
		newPod.Annotations[revisionKeys.TargetCustomVersionKey] = oldVal
	}
	if _, ok := tsMap[newRevision]; ok {
		if oldVal, ok := oldPod.Annotations[revisionKeys.TargetRevisionTimeStampKey]; ok {
			newPod.Annotations[revisionKeys.TargetRevisionTimeStampKey] = oldVal
		}
	}

	if newPod.Annotations[revisionKeys.CurrentRevisionKey] != newRevision {
		if cms.Spec.InjectUpdateOrder {
			if newPod.Labels == nil {
				newPod.Labels = make(map[string]string)
			}
			delete(newPod.Labels, asyncUpdateLabel)
		}
	}
	return nil
}

func restorePod(p *v1.Pod, cms *appsv1alpha1.ConfigMapSet, revisionKeys *RevisionKeys, tsMap map[string]*metav1.Time, asyncUpdateLabel string) error {
	return updatePod(p, revisionKeys, tsMap, cms, p.Annotations[revisionKeys.CurrentRevisionKey], p.Annotations[revisionKeys.CurrentCustomVersionKey], p.Annotations[revisionKeys.CurrentRevisionTimeStampKey], asyncUpdateLabel)
}

func (r *ReconcileConfigMapSet) HandlePodUpdate(ctx context.Context, c client.Client, p *v1.Pod, revisionKeys *RevisionKeys, newRevision string, tsMap map[string]*metav1.Time, cms *appsv1alpha1.ConfigMapSet, asyncUpdateLabel string, newCustomVersion string) (e error, shouldRetry bool, needRestart bool) {

	err := DeletePendingCRR(c, p)
	if err != nil {
		return err, false, false
	}

	err = updatePod(p, revisionKeys, tsMap, cms, newRevision, newCustomVersion, strconv.FormatInt(time.Now().UnixMilli(), 10), asyncUpdateLabel)
	if err != nil {
		klog.Errorf("failed to update pod %s/%s", p.Namespace, p.Name)
		return err, false, false
	}

	// 以下是pod重启的逻辑
	// 需要进行版本变更的pod, 需要进行并发度判断
	containersNeedToRestart := make([]string, 0) // 标记一下哪些容器需要重启
	// 如果没开restartInjectedContainers, 这里needToRestart只会有reload sidecar
	err, shouldRetry, containersNeedToRestart = r.updateContainers2Restart(p, cms, newRevision)
	if err != nil {
		// 不可以重启, 根据情况决定是否重试
		if shouldRetry {
			err2, updatedPod := rollbackAndSubmitPod(ctx, c, err, p, cms, revisionKeys, tsMap, newRevision, asyncUpdateLabel)
			if err2 != nil {
				return err2, false, false // rollback失败的情况下 不提交 不重试
			}
			// 提交成功, 要保存版本, 准备重试
			resourceVersionExpectations.Expect(updatedPod)
			return err, true, false
		}
		// 如果不需要重试 也不要提交pod的修改
		klog.Errorf("failed to update containers for pod %s/%s: %v, retry=%t", p.Namespace, p.Name, err, shouldRetry)
		return fmt.Errorf("failed to update containers for pod %s/%s: %v, retry=%t", p.Namespace, p.Name, err, shouldRetry), false, false
	}

	if len(containersNeedToRestart) == 0 {
		// err == nil, 但没有pod需要重启, 对应pod初次创建的情况
		return nil, false, false
	}

	// 重试创建CRR之前进行检查, 容器没有id会被webhook拒绝
	// 此时需要回滚
	for i := range p.Spec.Containers {
		container := &p.Spec.Containers[i]
		podContainer := util.GetContainer(container.Name, p)
		if podContainer == nil {
			return fmt.Errorf("container %s not found in Pod", container.Name), false, false
		}
		podContainerStatus := util.GetContainerStatus(container.Name, p)
		if podContainerStatus == nil {
			err2, updatedPod := rollbackAndSubmitPod(ctx, c, err, p, cms, revisionKeys, tsMap, newRevision, asyncUpdateLabel)
			if err2 != nil {
				return err2, false, false // rollback失败的情况下 不提交 不重试
			}
			// 提交成功, 要保存版本, 准备重试
			resourceVersionExpectations.Expect(updatedPod)
			klog.Warningf("not found %s containerStatus in Pod Status", container.Name)
			return fmt.Errorf("not found %s containerStatus in Pod Status", container.Name), true, false
		} else if podContainerStatus.ContainerID == "" {
			err2, updatedPod := rollbackAndSubmitPod(ctx, c, err, p, cms, revisionKeys, tsMap, newRevision, asyncUpdateLabel)
			if err2 != nil {
				return err2, false, false // rollback失败的情况下 不提交 不重试
			}
			// 提交成功, 要保存版本, 准备重试
			resourceVersionExpectations.Expect(updatedPod)
			klog.Warningf("no containerID in %s containerStatus, maybe the container has not been initialized", container.Name)
			return fmt.Errorf("no containerID in %s containerStatus, maybe the container has not been initialized", container.Name), true, false
		}
	}

	// 可以重启pod
	// 直接提交对pod的修改
	// 提交前打印当前pod的annotations
	klog.Infof("current pod %s/%s annotations: %+v", p.Namespace, p.Name, p.Annotations)
	updatedPod, err := submitPod(ctx, c, p)
	if err != nil {
		klog.Errorf("failed to submit pod %s/%s ", p.Namespace, p.Name)
		return err, false, false //  提交失败的情况下 不重试
	}
	// 提交成功, 要保存版本
	resourceVersionExpectations.Expect(updatedPod)

	if len(containersNeedToRestart) == 0 {
		klog.Infof("no containers need to restart in pod %s/%s: ", p.Namespace, p.Name)
		return nil, false, false
	}

	klog.Infof("containers need to restart in pod %s/%s: ", p.Namespace, p.Name)
	for _, name := range containersNeedToRestart {
		klog.Infof("%s", name)
	}

	// 创建ContainerRecreateRequests
	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cms-%s-%s", p.Name, uuid.New().String()),
			Namespace: p.Namespace,
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:    p.Name,
			Containers: make([]appsv1alpha1.ContainerRecreateRequestContainer, 0, len(containersNeedToRestart)),
			Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
				FailurePolicy:   appsv1alpha1.ContainerRecreateRequestFailurePolicyFail, // 一旦有某个容器停止或重建失败， CRR 立即结束
				OrderedRecreate: true,                                                   // 有序重建
			},
			TTLSecondsAfterFinished: int32Ptr(0),
		},
	}

	// 为需要重启的容器创建CRR
	for _, name := range containersNeedToRestart {
		crr.Spec.Containers = append(crr.Spec.Containers, appsv1alpha1.ContainerRecreateRequestContainer{
			Name: name,
		})
	}

	// 提交对CRR的创建
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		createErr := c.Create(context.TODO(), crr)
		if errors.IsAlreadyExists(createErr) {
			klog.Infof("CRR %s already exists, skipping creation.", crr.Name)
			return nil // 忽略错误
		}
		return createErr
	})

	if err != nil {
		klog.Errorf("failed to create CRR for pod %s/%s, error: %v", p.Namespace, p.Name, err)
		return fmt.Errorf("failed to create CRR for pod %s/%s, error: %v", p.Namespace, p.Name, err), false, false
	}

	return nil, false, true
}

func rollbackAndSubmitPod(ctx context.Context, c client.Client, err error, p *v1.Pod, cms *appsv1alpha1.ConfigMapSet, revisionKeys *RevisionKeys, tsMap map[string]*metav1.Time, newRevision string, asyncUpdateLabel string) (error, *v1.Pod) {
	// 把前面的修改回滚, 重试
	if err = rollbackPod(ctx, c, p, cms, revisionKeys, tsMap, newRevision, asyncUpdateLabel); err != nil {
		klog.Errorf("failed to rollback pod %s/%s ", p.Namespace, p.Name)
		return fmt.Errorf("failed to rollback pod %s/%s ", p.Namespace, p.Name), nil
	}
	// 打印rollback结束后的anno
	klog.Infof("after rollback, pod %s/%s annotation = %+v", p.Namespace, p.Name, p.Annotations)
	// 回滚成功了, 需要提交修改
	updatedPod, err := submitPod(ctx, c, p)
	if err != nil {
		klog.Errorf("failed to submit pod %s/%s ", p.Namespace, p.Name)
		return fmt.Errorf("failed to submit pod %s/%s ", p.Namespace, p.Name), nil
	}
	return nil, updatedPod
}

func int32Ptr(i int32) *int32 {
	return &i
}

func (r *ReconcileConfigMapSet) updateContainers2Restart(pod *v1.Pod, cms *appsv1alpha1.ConfigMapSet, newRevision string) (err error, shouldRetry bool, needToRestart []string) {
	klog.Infof("Updating containers to restart in Pod %s/%s for cms %s", pod.Namespace, pod.Name, cms.Name)
	// 如果当前pod的创建时间早于lastContainersTimeStamp
	// 说明spec.containers发生了变化
	// 则不进行后续的重启操作
	if pod.Status.StartTime.Before(cms.Status.LastContainersTimestamp) {
		klog.Infof("pod %s/%s is created before last change of InjectedContainers, skip restart", pod.Namespace, pod.Name)
		r.recorder.Eventf(pod, v1.EventTypeNormal, "SkipRestart", "pod is created before last change of cms %s/%s's InjectedContainers, skip restart", cms.Namespace, cms.Name)
		return nil, false, nil
	}
	// 先找到reload-sidecar
	sidecarName := cms.Name + "-reload-sidecar"
	var reloadSidecarCS *v1.ContainerStatus
	for _, v := range pod.Status.ContainerStatuses {
		if v.Name == sidecarName {
			reloadSidecarCS = v.DeepCopy()
			break
		}
	}
	// 如果没有找到, 说明inject有问题
	if reloadSidecarCS == nil {
		klog.Errorf("container reload-sidecar not found in pod %s/%s for cms %s", pod.Namespace, pod.Name, cms.Name)
		return fmt.Errorf("container reload-sidecar not found in pod %s/%s for cms %s", pod.Namespace, pod.Name, cms.Name), false, nil
	}

	targetRevisionKey := AnnotationPrefix + cms.Name + AnnotationTargetRevisionSuffix

	// 判断reload-sidecar容器重启时间
	ts, exist := pod.Annotations[targetRevisionKey+"Timestamp"]
	// 新创建的pod 该annotation值为0, 不用走重启的逻辑
	if exist && ts != "0" {
		klog.V(2).Infof("Triggering reload-sidecar (%s) in Pod %s/%s to restart, because the pod has inconsistent hash of configmapset revision", reloadSidecarCS.ContainerID, pod.Namespace, pod.Name)
		klog.Infof("old revision: %s, new revision: %s", pod.Annotations[targetRevisionKey], newRevision)
	} else {
		klog.Infof("pod first created, no need to restart reload-sidecar for pod %s/%s", pod.Namespace, pod.Name)
		return nil, false, nil
	}

	// 现在确定可以重启
	// reload-sidecar需要重启
	needToRestart = append(needToRestart, reloadSidecarCS.Name) // 重启reload-sidecar
	// 判断cms是否开启了injectedContainers
	if cms.Spec.UpdateStrategy.RestartInjectedContainers {
		// 要把相关容器全都重启
		for _, c := range cms.Spec.InjectedContainers {
			found := false
			klog.Infof("judging whether to restart container %s in pod %s/%s for cms %s", c.Name, pod.Namespace, pod.Name, cms.Name)
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == c.Name {
					found = true
					needToRestart = append(needToRestart, cs.Name)
				}
			}
			if !found {
				klog.Errorf("container %s status not found in pod %s/%s for cms %s", c.Name, pod.Namespace, pod.Name, cms.Name)
				continue
			}
		}
	}

	return nil, false, needToRestart
}

func DeletePendingCRR(c client.Client, pod *v1.Pod) error {
	// 需要进行版本变更的pod, 可能涉及CRR的创建, 需要把历史上Pending的都删掉
	crrList := &appsv1alpha1.ContainerRecreateRequestList{}
	err := c.List(context.TODO(), crrList, client.InNamespace(pod.Namespace), client.MatchingLabels{appsv1alpha1.ContainerRecreateRequestPodUIDKey: string(pod.UID)})
	if err != nil {
		klog.Errorf("Failed to get CRR List for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return fmt.Errorf("failed to get CRR List for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}

	if len(crrList.Items) > 0 {
		// 说明存在历史CRR, 需要删掉
		for i := range crrList.Items {
			crr := &crrList.Items[i]
			if crr.Spec.PodName != pod.Name || crr.Status.Phase != appsv1alpha1.ContainerRecreateRequestPending {
				// 当前只处理Pending的情况
				continue
			}
			klog.Infof("deleting CRR for Pod %s/%s", pod.Namespace, pod.Name)
			// 提交对CRR的删除
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				createErr := c.Delete(context.TODO(), crr)
				return createErr
			})
			if err != nil {
				klog.Errorf("Failed to delete CRR for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
			}
		}
	}
	return nil
}

func HandleUpdatedPod(ctx context.Context, c client.Client, revisionKeys *RevisionKeys, tsMap map[string]*metav1.Time, cms *appsv1alpha1.ConfigMapSet, asyncUpdateLabel string, p *v1.Pod) (err error, shouldRetry bool) {
	klog.Infof("Processing pod %s/%s", p.Namespace, p.Name)
	updated := false

	// 新创建的pod没有targetRevisionTSKey, 需要添加
	// 回滚的场景下, timestamp可能会不一样(因为可以存在revision相同而customVersion不同的情况), 不作更新
	// revisionTimestamp 预期只在两个地方更新
	// 一个是pod刚创建的时候(injectSidecar)
	// 一个是pod revision发生变化的时候
	if _, exists := p.Annotations[revisionKeys.TargetRevisionTimeStampKey]; !exists {
		// 预期每个pod的该annotation都不应该为空
		klog.Errorf("Annotation %s should not be empty for pod %s/%s", revisionKeys.TargetRevisionTimeStampKey, p.Namespace, p.Name)
		return fmt.Errorf("annotation %s should not be empty for pod %s/%s", revisionKeys.TargetRevisionTimeStampKey, p.Namespace, p.Name), false
	}

	// 首先判断当前pod有无CRR存在, 如果有, 先跳过当前流程, 等重试
	// 需要进行版本变更的pod, 可能涉及CRR的创建, 需要把历史上Pending的都删掉
	crrList := &appsv1alpha1.ContainerRecreateRequestList{}
	err = c.List(context.TODO(), crrList, client.InNamespace(p.Namespace), client.MatchingLabels{appsv1alpha1.ContainerRecreateRequestPodUIDKey: string(p.UID)})
	if err != nil {
		klog.Errorf("Failed to get CRR List for Pod %s/%s: %v", p.Namespace, p.Name, err)
		return fmt.Errorf("failed to get CRR List for Pod %s/%s: %v", p.Namespace, p.Name, err), false
	}

	if len(crrList.Items) > 0 {
		// 存在CRR, 等重试
		klog.Infof("pod %s/%s has CRR, skip", p.Namespace, p.Name)
		return nil, true
	}

	// 如果没有设置RestartInjectedContainers, 可以不用管, 已经是预期状态了
	if !cms.Spec.UpdateStrategy.RestartInjectedContainers {
		klog.Infof("pod %s/%s has not set RestartInjectedContainers, skip", p.Namespace, p.Name)
		return nil, false
	}

	// 需要判断相关容器是否已经重启好了
	podStatus := p.Status
	// 只要容器的启动时间都晚于触发该pod更新的cms最后修改时间时间(即revisionTimeStamp)就可以认为重启完成了
	// 检查是否所有相关容器的重启时间都晚于revisionTimeStamp
	allContainersRestarted := true

	// 判断相关容器的重启时间
	for _, injectedContainer := range cms.Spec.InjectedContainers {
		// 在pod中找同名容器 container
		for _, containerStatus := range podStatus.ContainerStatuses {
			if containerStatus.Name == injectedContainer.Name {
				// 判断容器的状态
				// 如果不是Running, 说明容器可能处于启动过程中, 等待启动完成再次触发update处理
				if containerStatus.State.Running == nil {
					allContainersRestarted = false
					klog.Infof("container %s is not running for cms %s, pod %s/%s", injectedContainer.Name, cms.Name, p.Namespace, p.Name)
					break
				}
				restartTS := containerStatus.State.Running.StartedAt.UnixMilli()
				revisionTS, err := strconv.ParseInt(p.Annotations[revisionKeys.TargetRevisionTimeStampKey], 10, 64)
				if err != nil {
					klog.Errorf("parse revision timestamp failed %v:", err)
					return err, false
				}

				if restartTS <= revisionTS {
					// 获取容器的最近重启时间（秒级时间戳）
					// 如果容器的重启时间 晚于 revisionTimeStamp的时间
					// 则认为已重启完成
					allContainersRestarted = false
					klog.Infof("container %s restartTime is earlier than target revision timestamp for cms %s, pod %s/%s", injectedContainer.Name, cms.Name, p.Namespace, p.Name)
					klog.Infof("restart time: %d, target revision timestamp: %d", restartTS, revisionTS)
					break
				}
			}
		}
	}

	// 这里存在一种情况, 就是currentVersion与targetVersion不一致, 可能是因为之前的cms没有选择RestartInjectedContainers
	// 所以也没有CRR
	// 此时就需要把targetVersion重置回customVersion, 让cms重新reconcile
	// 如果没有重启好, 同时也没有CRR -> 回调版本
	if !allContainersRestarted {
		// 如果已经一致的话, 可能是回滚, 不用处理
		if p.Annotations[revisionKeys.TargetRevisionKey] != p.Annotations[revisionKeys.CurrentRevisionKey] {
			err = restorePod(p, cms, revisionKeys, tsMap, asyncUpdateLabel)
			if err != nil {
				return err, false
			}
			updatedPod, err := submitPod(ctx, c, p)
			if err != nil {
				klog.Errorf("failed to submit pod %s/%s ", p.Namespace, p.Name)
				return err, false //  提交失败的情况下 不重试
			}
			// 提交成功, 要保存版本
			resourceVersionExpectations.Expect(updatedPod)
			return nil, true // 重试
		}
	}

	// 如果cms.Spec.InjectedContainers中每个容器的重启时间都要晚于revisionTimeStamp
	// 则认为已经更新完成, 执行以下操作
	klog.Infof("all containers restarted for cms %s, pod %s/%s", cms.Name, p.Namespace, p.Name)
	if p.Annotations == nil {
		p.Annotations = make(map[string]string)
	}
	// 只有在 revision 发生变更时才更新
	if p.Annotations[revisionKeys.CurrentRevisionKey] != p.Annotations[revisionKeys.TargetRevisionKey] {
		klog.Infof("updating current annotation for cms %s, pod %s/%s", cms.Name, p.Namespace, p.Name)
		p.Annotations[revisionKeys.CurrentRevisionKey] = p.Annotations[revisionKeys.TargetRevisionKey]
		p.Annotations[revisionKeys.CurrentCustomVersionKey] = p.Annotations[revisionKeys.TargetCustomVersionKey]
		p.Annotations[revisionKeys.CurrentRevisionTimeStampKey] = strconv.FormatInt(time.Now().UnixMilli(), 10) // 记录当前时间
		// 去掉label asyncUpdateLabel
		if p.Labels != nil {
			if _, exists := p.Labels[asyncUpdateLabel]; exists {
				klog.Infof("removing label %s for cms %s, pod %s/%s", asyncUpdateLabel, cms.Name, p.Namespace, p.Name)
				delete(p.Labels, asyncUpdateLabel)
				updated = true
			}
		}
		updated = true
	}

	if updated {
		maxRetries := 5
		for i := 0; i < maxRetries; i++ {
			err := c.Update(ctx, p)
			if err == nil {
				break
			}
			if apierrors.IsConflict(err) {
				klog.Warningf("Retrying pod update %s/%s due to conflict (attempt %d)", p.Namespace, p.Name, i+1)
				time.Sleep(time.Millisecond * 100)                                        // 短暂等待，避免高并发冲突
				c.Get(ctx, types.NamespacedName{Name: p.Name, Namespace: p.Namespace}, p) // 重新获取最新的 pod 状态
				continue
			}
			return err, false
		}
	}
	// 没更新直接跳过即可
	return nil, false
}

func notExistInDistribution(rev string, distributionsCopy []appsv1alpha1.Distribution) bool {
	for _, d := range distributionsCopy {
		if d.Revision == rev {
			return false
		}
	}
	return true
}

// updateStatus 更新cms的status字段
func (r *ReconcileConfigMapSet) updateStatus(ctx context.Context, request reconcile.Request, cms *appsv1alpha1.ConfigMapSet, hashChanged bool) error {
	klog.Infof("Updating status for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	// 获取关联Pod列表
	pods, err := GetMatchedPods(ctx, r.Client, cms)
	if err != nil {
		return err
	}
	var updateRevision string
	var updatedPodsNum, readyPodsNum, updatedReadyPodsNum int32
	// apps.kruise.io/configmapset.configMapSet名称.revision
	targetRevisionKey := AnnotationPrefix + cms.Name + AnnotationTargetRevisionSuffix
	currentRevisionKey := AnnotationPrefix + cms.Name + AnnotationCurrentRevisionSuffix

	updateRevision, err = CalculateHash(cms.Spec.Data) // 计算当前版本
	if err != nil {
		return fmt.Errorf("failed to compute hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	// 计算状态指标
	for _, pod := range pods {
		if pod.Annotations[targetRevisionKey] == updateRevision && pod.Annotations[currentRevisionKey] == updateRevision {
			updatedPodsNum++
			if IsPodReady(pod) {
				klog.Infof("Pod %s/%s is ready", pod.Namespace, pod.Name)
				readyPodsNum++
				updatedReadyPodsNum++
			}
		} else if IsPodReady(pod) {
			klog.Infof("Pod %s/%s is ready", pod.Namespace, pod.Name)
			readyPodsNum++
		}
	}

	klog.Infof("cms %s matched pods: %d, ready pods: %d, updated pods: %d, updated ready pods: %d", cms.Name, len(pods), readyPodsNum, updatedPodsNum, updatedReadyPodsNum)

	if cms.Status.ObservedGeneration == cms.Generation &&
		cms.Status.UpdateRevision == updateRevision &&
		cms.Status.ReadyPods == readyPodsNum &&
		cms.Status.MatchedPods == int32(len(pods)) &&
		cms.Status.UpdatedPods == updatedPodsNum &&
		cms.Status.UpdatedReadyPods == updatedReadyPodsNum && !hashChanged {
		klog.Infof("No change in status for ConfigMapSet %s/%s, skipping update", cms.Namespace, cms.Name)
		return nil
	}

	// 更新ConfigMapSet状态
	// 所有pod都是最新版以后才能更新currentRevision
	// 如果cms是第一次发版, 就使用当前版本
	if updatedReadyPodsNum == int32(len(pods)) || cms.Status.CurrentRevision == "" {
		cms.Status.CurrentRevision = updateRevision
		// 仅在所有pod都更新到一个版本以后进行版本维护
		err = r.cleanHistoryRevision(ctx, cms)
	}
	cms.Status.ObservedGeneration = cms.Generation
	cms.Status.UpdateRevision = updateRevision
	cms.Status.MatchedPods = int32(len(pods))
	cms.Status.ReadyPods = readyPodsNum
	cms.Status.UpdatedPods = updatedPodsNum
	cms.Status.UpdatedReadyPods = updatedReadyPodsNum

	// 将更新同步到 etcd
	klog.Infof("Updating cms %s status %#v", cms.Name, cms.Status)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// 获取最新的对象
		latest := &appsv1alpha1.ConfigMapSet{}
		if err := r.Get(context.TODO(), request.NamespacedName, latest); err != nil {
			utilruntime.HandleError(fmt.Errorf("error getting latest configmapset %s/%s: %v", cms.Namespace, cms.Name, err))
			return err
		}

		// 更新 Status 字段
		latest.Status = cms.Status

		fmt.Println("Before update, RV:", latest.ResourceVersion)

		// 执行 Status 更新
		if err := r.Status().Update(context.TODO(), latest); err != nil {
			utilruntime.HandleError(fmt.Errorf("error updating configmapset status %s/%s: %v", cms.Namespace, cms.Name, err))
			return err
		}

		// 打印更新后的 ResourceVersion
		klog.Infof("Updated cms %s status successfully, new ResourceVersion: %s", latest.Name, latest.ResourceVersion)
		return nil
	})
}

func (r *ReconcileConfigMapSet) cleanHistoryRevision(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	// ConfigMap 命名：cms.Name + "-hub"
	cmName := fmt.Sprintf("%s-hub", cms.Name)
	cmNamespace := cms.Namespace

	var revisions []RevisionEntry

	// 定义更新逻辑
	updateFunc := func() error {
		// 重新获取最新的 ConfigMap，避免并发冲突
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
			if errors.IsNotFound(err) {
				// 如果 ConfigMap 不存在，则创建
				newCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmName,
						Namespace: cmNamespace,
					},
					Data: make(map[string]string),
				}
				return r.Create(ctx, newCM)
			}
			return fmt.Errorf("failed to get ConfigMap: %v", err)
		}

		// 解析现有 ConfigMap 的 revisions
		if revData, exists := cm.Data["revisions"]; exists {
			if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
				klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
				return fmt.Errorf("failed to unmarshal revisions from ConfigMap %s: %v", cmName, err)
			}
		}

		// 维护 RevisionHistoryLimit
		// 保留最新的 RevisionHistoryLimit 个
		if cms.Spec.RevisionHistoryLimit != nil && int32(len(revisions)) > *cms.Spec.RevisionHistoryLimit {
			klog.Infof("Trimming old revisions to match RevisionHistoryLimit (%d)", *cms.Spec.RevisionHistoryLimit)
			keep := int(*cms.Spec.RevisionHistoryLimit)
			start := len(revisions) - keep
			if start < 0 {
				start = 0
			}
			revisions = revisions[start:] // 要保留的版本
		}

		klog.Infof("Updated revisions for ConfigMapSet %s/%s: %v", cms.Namespace, cms.Name, revisions)

		// 更新 ConfigMap Data
		revBytes, err := json.Marshal(revisions)
		if err != nil {
			return fmt.Errorf("failed to marshal revisions: %v", err)
		}
		cm = cm.DeepCopy() // 避免修改缓存对象
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data["revisions"] = string(revBytes)

		// 更新 ConfigMap
		return r.Update(ctx, cm)
	}

	// 使用 `retry.RetryOnConflict` 处理更新冲突
	err := retry.RetryOnConflict(retry.DefaultRetry, updateFunc)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap %s: %v", cmName, err)
	}
	return nil
}

func (r *ReconcileConfigMapSet) syncPods(ctx context.Context, cms *appsv1alpha1.ConfigMapSet, entry []RevisionEntry, pods []*corev1.Pod) (error, bool) {
	return r.SyncPods(ctx, r.Client, cms, entry, pods)
}
