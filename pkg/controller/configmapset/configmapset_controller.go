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
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	corev1scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/fieldpath"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runController "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "configmapset-workers", concurrentReconciles, "Max concurrent workers for ConfigMapSet controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("ConfigMapSet")
)

type RevisionKeys struct {
	CurrentRevisionKey          string
	CurrentRevisionTimeStampKey string
	CurrentCustomVersionKey     string
	UpdateRevisionKey           string
	UpdateRevisionTimeStampKey  string
	UpdateCustomVersionKey      string
}

type SpecForHash struct {
	CustomVersion        string                             `json:"customVersion,omitempty"`
	Selector             *metav1.LabelSelector              `json:"selector"`
	Data                 map[string]string                  `json:"data"`
	Containers           []appsv1alpha1.ContainerInjectSpec `json:"containers"`
	ReloadSidecarConfig  *appsv1alpha1.ReloadSidecarConfig  `json:"reloadSidecarConfig,omitempty"`
	EffectPolicy         *appsv1alpha1.EffectPolicy         `json:"effectPolicy,omitempty"`
	RevisionHistoryLimit *int32                             `json:"revisionHistoryLimit,omitempty"`
}

type UpdateInfo struct {
	PodsToUpdate         []*corev1.Pod
	TargetRevisions      []string
	TargetCustomVersions []string
}

const (
	ConfigMapFinalizerName = "finalizer.configmapset.kruise.io"
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

type ReconcileConfigMapSet struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	config   *rest.Config
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("configmapset-controller")
	return &ReconcileConfigMapSet{
		Client:   utilclient.NewClientFromManager(mgr, "configmapset-controller"),
		scheme:   mgr.GetScheme(),
		recorder: recorder,
		config:   mgr.GetConfig(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := runController.New("configmapset-controller", mgr, runController.Options{
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
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// handle delete
	if cms.DeletionTimestamp != nil {
		if containsString(cms.Finalizers, ConfigMapFinalizerName) {
			// 1. check has injected pod exist by webhook
			// 2. clean configmap-hub
			if err := r.cleanupConfigMap(ctx, cms); err != nil {
				return reconcile.Result{}, fmt.Errorf("cleanupConfigMap failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
			}
			// 3. remove finalizer
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				latest := &appsv1alpha1.ConfigMapSet{}
				if err = r.Get(ctx, types.NamespacedName{Name: cms.Name, Namespace: cms.Namespace}, latest); err != nil {
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

	// manage configmap-hub
	err = r.syncRevisions(ctx, cms)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("manageRevisions failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	pods, err := GetMatchedPods(ctx, r.Client, cms)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("GetMatchedPods failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	// Pod同步
	requeue, err := r.SyncPods(ctx, cms, pods)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("syncPods failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	if requeue {
		klog.Infof("ConfigMapSet %s/%s needs requeue to wait for pod updates", cms.Namespace, cms.Name)
		return reconcile.Result{RequeueAfter: 3 * time.Second}, nil
	}

	// 更新状态
	err = r.updateStatus(ctx, request, cms)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("updateStatus failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileConfigMapSet) cleanupConfigMap(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	cmName := fmt.Sprintf("%s-%s", strings.ToLower(cms.Name), "hub")
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cms.Namespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
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
func (r *ReconcileConfigMapSet) syncRevisions(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	hash, err := CalculateHash(cms.Spec.Data)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %v", err)
	}

	// ConfigMap 命名：cms.Name + "-hub"
	cmName := fmt.Sprintf("%s-%s", strings.ToLower(cms.Name), "hub")
	cmNamespace := cms.Namespace
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm := &corev1.ConfigMap{}
		if err = r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
			if errors.IsNotFound(err) {
				err = r.Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmName,
						Namespace: cmNamespace,
					},
					Data: make(map[string]string),
				})
				if err != nil {
					return fmt.Errorf("failed to create ConfigMap %s/%s: %v", cmNamespace, cmName, err)
				}
				return fmt.Errorf("create ConfigMap %s/%s: %v", cmNamespace, cmName, err)
			}
			return fmt.Errorf("failed to get ConfigMap: %v", err)
		}

		var revisions []RevisionEntry
		if revData, exists := cm.Data["revisions"]; exists {
			if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
				klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
				return fmt.Errorf("failed to unmarshal revisions from ConfigMap %s: %v", cmName, err)
			}
		}

		for _, rev := range revisions {
			if rev.Hash == hash && rev.CustomVersion == cms.Spec.CustomVersion {
				klog.Warningf("Revision %s already exists in ConfigMap %s, skipping update", hash, cmName)
				return nil
			}
		}
		revisions = append(revisions, RevisionEntry{
			Hash:          hash,
			TimeStamp:     &metav1.Time{Time: time.Now()},
			Data:          cms.Spec.Data,
			CustomVersion: cms.Spec.CustomVersion,
		})

		klog.Infof("Updated revisions for ConfigMapSet %s/%s: %v", cms.Namespace, cms.Name, revisions)
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
	})
}

func (r *ReconcileConfigMapSet) SyncPods(ctx context.Context, cms *appsv1alpha1.ConfigMapSet, pods []*corev1.Pod) (bool, error) {
	klog.Infof("Syncing pods for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	revisionKeys := &RevisionKeys{
		CurrentRevisionKey:          GetConfigMapSetCurrentRevisionKey(cms.Name),
		CurrentCustomVersionKey:     GetConfigMapSetCurrentCustomVersionKey(cms.Name),
		CurrentRevisionTimeStampKey: GetConfigMapSetCurrentRevisionTimeStampKey(cms.Name),
		UpdateRevisionKey:           GetConfigMapSetUpdateRevisionKey(cms.Name),
		UpdateCustomVersionKey:      GetConfigMapSetUpdateCustomVersionKey(cms.Name),
		UpdateRevisionTimeStampKey:  GetConfigMapSetUpdateRevisionTimeStampKey(cms.Name),
	}
	// 根据更新策略选择要更新的Pod
	updateStrategy := cms.Spec.UpdateStrategy

	requeue := false

	// 定义了Partition,就只更新replicas-partition个Pod实例，没定义就是全部需要更新
	if updateStrategy.Partition != nil {
		// Group pods by matchLabelKeys
		var podGroups [][]*corev1.Pod
		if len(updateStrategy.MatchLabelKeys) == 0 {
			podGroups = append(podGroups, pods)
		} else {
			groupMap := make(map[string][]*corev1.Pod)
			for _, pod := range pods {
				keyVals := make([]string, len(updateStrategy.MatchLabelKeys))
				for i, key := range updateStrategy.MatchLabelKeys {
					keyVals[i] = pod.Labels[key]
				}
				groupKey := strings.Join(keyVals, "|")
				groupMap[groupKey] = append(groupMap[groupKey], pod)
			}
			// Map iteration order is non-deterministic, so sort keys for consistent grouping
			var keys []string
			for k := range groupMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				podGroups = append(podGroups, groupMap[k])
			}
		}

		var allErrs []error
		for _, group := range podGroups {
			distributions, err := getDistributionByPartition(cms, group)
			if err != nil {
				klog.Errorf("failed to transform partition to distribution: %v", err)
				allErrs = append(allErrs, fmt.Errorf("failed to transform partition to distribution: %v", err))
				continue
			}
			groupRequeue, err := r.UpdateByDistribution(ctx, cms, distributions, group, revisionKeys)
			if groupRequeue {
				requeue = true
			}
			if err != nil {
				allErrs = append(allErrs, err)
			}
		}

		if len(allErrs) > 0 {
			return requeue, fmt.Errorf("errors occurred during syncPods: %v", allErrs)
		}
		return requeue, nil
	}
	return requeue, nil
}

// Distribution is used internally in controller now
type Distribution struct {
	Revision      string `json:"revision,omitempty"`
	CustomVersion string `json:"customVersion,omitempty"`
	Reserved      int32  `json:"reserved"`
}

func getDistributionByPartition(cms *appsv1alpha1.ConfigMapSet, pods []*corev1.Pod) ([]Distribution, error) {
	// 总副本数 = 传入的pod数
	replicas := len(pods)
	// 解析 partition
	partition := cms.Spec.UpdateStrategy.Partition

	// 由于syncPods在updateStatus之前, 这里需要计算当前最新的updateRevision
	updateRevision, err := CalculateHash(cms.Spec.Data) // 计算当前版本
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}
	updateCustomVersion := cms.Spec.CustomVersion
	currentRevision := cms.Status.CurrentRevision
	currentCustomVersion := cms.Status.CurrentCustomVersion

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
			newReplicas = replicas - int(math.Ceil(float64(percent)/100.0*float64(replicas)))
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
	distributions := make([]Distribution, 2)
	distributions[0] = Distribution{
		Revision:      updateRevision,
		Reserved:      int32(newReplicas),
		CustomVersion: updateCustomVersion,
	}
	distributions[1] = Distribution{
		Revision:      currentRevision,
		Reserved:      int32(replicas - newReplicas),
		CustomVersion: currentCustomVersion,
	}

	klog.Infof("distributions after transform: %v", distributions)
	return distributions, nil
}

func getUpdateInfoByDistribution(distributions []Distribution, pods []*corev1.Pod, revisionKeys *RevisionKeys) (*UpdateInfo, error) {
	// 收集待更新 pod（多余版本 -> 目标版本）
	var podsToUpdate []*corev1.Pod
	targetRevisions := make([]string, 0)
	targetCustomVersions := make([]string, 0)

	podsToUpdate, targetRevisions, targetCustomVersions = getUpdatePodsByDistributions(distributions, pods, revisionKeys)
	return &UpdateInfo{
		PodsToUpdate:         podsToUpdate,
		TargetRevisions:      targetRevisions,
		TargetCustomVersions: targetCustomVersions,
	}, nil
}

func getUpdatePodsByDistributions(distributions []Distribution, pods []*v1.Pod, revisionKeys *RevisionKeys) ([]*v1.Pod, []string, []string) {
	updateDistribution := distributions[0]
	currentDistribution := distributions[1]

	var validPods []*v1.Pod
	var podsToUpdate []*v1.Pod
	var targetRevisions []string
	var targetCustomVersions []string

	// Filter pods that don't have the enabled annotation
	for _, pod := range pods {
		if pod.Annotations != nil && pod.Annotations[GetConfigMapSetEnabledKey()] == "true" {
			validPods = append(validPods, pod)
		}
	}

	// Calculate how many pods are currently at the update revision
	var currentUpdateRevisionCount int32 = 0
	for _, pod := range validPods {
		if pod.Annotations != nil && pod.Annotations[revisionKeys.CurrentRevisionKey] == updateDistribution.Revision {
			currentUpdateRevisionCount++
		}
	}

	// Calculate how many pods we need to update
	needToUpdate := updateDistribution.Reserved - currentUpdateRevisionCount
	if needToUpdate <= 0 {
		return podsToUpdate, targetRevisions, targetCustomVersions
	}

	// Sort valid pods based on the priority rules
	sort.Slice(validPods, func(i, j int) bool {
		podI := validPods[i]
		podJ := validPods[j]

		revI := ""
		revJ := ""
		if podI.Annotations != nil {
			revI = podI.Annotations[revisionKeys.CurrentRevisionKey]
		}
		if podJ.Annotations != nil {
			revJ = podJ.Annotations[revisionKeys.CurrentRevisionKey]
		}

		// 1. Non-currentRevision > currentRevision
		isICurrent := revI == currentDistribution.Revision
		isJCurrent := revJ == currentDistribution.Revision
		if !isICurrent && isJCurrent {
			return true
		}
		if isICurrent && !isJCurrent {
			return false
		}

		// At this point, both are either currentRevision or both are non-currentRevision
		// For non-currentRevision pods, apply rules 2 and 3
		if !isICurrent && !isJCurrent {
			// In a real implementation, we would check if they exist in RMC and compare their generation
			// For simplicity without having the RMC context here, we can fall back to creation timestamp
			// or other deterministic properties if needed, but we'll try to follow the spirit of the rules

			// If we had RMC info:
			// 3. Not in RMC > in RMC (we don't have RMC info here directly)
			// 2. Smaller generation > larger generation (we can use ResourceVersion or CreationTimestamp as a proxy if generation is not available)
		}
		if podI.CreationTimestamp.Equal(&podJ.CreationTimestamp) {
			return podI.Name < podJ.Name
		}

		// Deterministic fallback: older pods get updated first
		return podI.CreationTimestamp.Before(&podJ.CreationTimestamp)
	})

	// Select pods to update
	for _, pod := range validPods {
		if needToUpdate <= 0 {
			break
		}

		rev := ""
		if pod.Annotations != nil {
			rev = pod.Annotations[revisionKeys.CurrentRevisionKey]
		}

		// Skip pods already at the update revision
		if rev == updateDistribution.Revision {
			continue
		}

		podsToUpdate = append(podsToUpdate, pod)
		targetRevisions = append(targetRevisions, updateDistribution.Revision)
		targetCustomVersions = append(targetCustomVersions, updateDistribution.CustomVersion)
		needToUpdate--
	}

	return podsToUpdate, targetRevisions, targetCustomVersions
}

// execInContainer executes a command in the specified pod and container
func (r *ReconcileConfigMapSet) execInContainer(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string) (string, string, error) {
	clientset, err := kubernetes.NewForConfig(r.config)
	if err != nil {
		return "", "", err
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, corev1scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(r.config, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	return stdout.String(), stderr.String(), err
}

func (r *ReconcileConfigMapSet) verifySharedVolumeUpdated(ctx context.Context, pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	containerName := r.getReloadSidecarName(ctx, cms)
	configMountPath := fmt.Sprintf("%s/%s", "/etc/config", strings.ToLower(cms.Name))

	for key, expectedVal := range cms.Spec.Data {
		cmd := []string{"cat", fmt.Sprintf("%s/%s", configMountPath, key)}
		stdout, _, err := r.execInContainer(ctx, pod, containerName, cmd)
		if err != nil {
			return fmt.Errorf("failed to exec cat %s: %v", key, err)
		}

		if stdout != expectedVal {
			return fmt.Errorf("file %s content does not match expected", key)
		}
	}
	return nil
}

func (r *ReconcileConfigMapSet) resolvePort(pod *corev1.Pod, containerName string, port intstr.IntOrString) (int, error) {
	if port.Type == intstr.Int {
		return port.IntValue(), nil
	}
	for _, c := range pod.Spec.Containers {
		if c.Name == containerName {
			for _, p := range c.Ports {
				if p.Name == port.StrVal {
					return int(p.ContainerPort), nil
				}
			}
		}
	}
	return 0, fmt.Errorf("port %s not found in container %s", port.StrVal, containerName)
}

func (r *ReconcileConfigMapSet) executePostHook(ctx context.Context, pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	if cms.Spec.EffectPolicy == nil || cms.Spec.EffectPolicy.PostHook == nil {
		return nil
	}

	hook := cms.Spec.EffectPolicy.PostHook

	if len(cms.Spec.Containers) == 0 {
		return fmt.Errorf("no business containers defined in ConfigMapSet")
	}

	for _, containerSpec := range cms.Spec.Containers {
		containerName := GetContainerName(pod, containerSpec)
		if containerName == "" {
			klog.Warningf("Pod %s/%s cannot determine container name from spec %v in executePostHook, skipping", pod.Namespace, pod.Name, containerSpec)
			continue
		}

		if hook.Exec != nil {
			stdout, stderr, err := r.execInContainer(ctx, pod, containerName, hook.Exec.Command)
			if err != nil {
				return fmt.Errorf("exec hook failed for container %s: %v, stderr: %s", containerName, err, stderr)
			}
			klog.Infof("Exec hook for pod %s/%s container %s output: %s", pod.Namespace, pod.Name, containerName, stdout)
		}

		if hook.HTTPGet != nil {
			port, err := r.resolvePort(pod, containerName, hook.HTTPGet.Port)
			if err != nil {
				return err
			}

			scheme := string(hook.HTTPGet.Scheme)
			if scheme == "" {
				scheme = "http"
			}

			url := fmt.Sprintf("%s://%s:%d%s", strings.ToLower(scheme), pod.Status.PodIP, port, hook.HTTPGet.Path)

			httpClient := &http.Client{Timeout: 5 * time.Second}
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return err
			}

			for _, h := range hook.HTTPGet.HTTPHeaders {
				req.Header.Add(h.Name, h.Value)
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("http get failed for container %s: %v", containerName, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				return fmt.Errorf("http get returned status code %d for container %s", resp.StatusCode, containerName)
			}
		}

		if hook.TCPSocket != nil {
			port, err := r.resolvePort(pod, containerName, hook.TCPSocket.Port)
			if err != nil {
				return err
			}

			address := net.JoinHostPort(pod.Status.PodIP, strconv.Itoa(int(port)))
			conn, err := net.DialTimeout("tcp", address, 5*time.Second)
			if err != nil {
				return fmt.Errorf("tcp socket failed for container %s: %v", containerName, err)
			}
			conn.Close()
		}
	}

	return nil
}

func (r *ReconcileConfigMapSet) getReloadSidecarName(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) string {
	expectedSidecarName := fmt.Sprintf("%s-%s", strings.ToLower(cms.Name), "reload-sidecar")
	if cms.Spec.ReloadSidecarConfig != nil {
		if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.K8sConfigReloadSidecarType && cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.Name != "" {
			expectedSidecarName = cms.Spec.ReloadSidecarConfig.Config.Name
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.SidecarSetReloadSidecarType && cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.SidecarSetRef != nil {
			expectedSidecarName = cms.Spec.ReloadSidecarConfig.Config.SidecarSetRef.ContainerName
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.CustomerReloadSidecarType {
			if cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef != nil {
				cmRef := cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef
				customerCM := &corev1.ConfigMap{}
				if err := r.Get(ctx, types.NamespacedName{Name: cmRef.Name, Namespace: cmRef.Namespace}, customerCM); err == nil {
					for _, v := range customerCM.Data {
						var reloadSidecar corev1.Container
						if unmarshalErr := json.Unmarshal([]byte(v), &reloadSidecar); unmarshalErr == nil {
							if reloadSidecar.Name != "" {
								expectedSidecarName = reloadSidecar.Name
							}
							break
						}
					}
				} else {
					klog.Errorf("failed to get customer sidecar configmap %s/%s: %v", cmRef.Namespace, cmRef.Name, err)
				}
			}
		}
	}
	return expectedSidecarName
}

func (r *ReconcileConfigMapSet) UpdateByDistribution(ctx context.Context, cms *appsv1alpha1.ConfigMapSet, distributions []Distribution, pods []*corev1.Pod, revisionKeys *RevisionKeys) (bool, error) {
	updateInfo, err := getUpdateInfoByDistribution(distributions, pods, revisionKeys)
	if err != nil {
		klog.Errorf("failed to fetch update info: %v", err)
		return false, err
	}

	requeue := false
	for i, pod := range updateInfo.PodsToUpdate {
		if pod == nil || !controller.IsPodActive(pod) {
			continue
		}
		targetRevision := updateInfo.TargetRevisions[i]
		targetCustomVersion := updateInfo.TargetCustomVersions[i]

		klog.Infof("Updating Pod %s/%s to revision %s", pod.Namespace, pod.Name, targetRevision)

		// 1. Update Pod Annotation (TargetRevision and TargetCustomVersion)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latestPod := &corev1.Pod{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, latestPod); err != nil {
				return err
			}

			if latestPod.Annotations == nil {
				latestPod.Annotations = make(map[string]string)
			}

			// If already updated CurrentRevision, skip
			if latestPod.Annotations[revisionKeys.CurrentRevisionKey] == targetRevision &&
				latestPod.Annotations[revisionKeys.CurrentCustomVersionKey] == targetCustomVersion {
				return nil
			}

			// If TargetRevision is not yet updated, update it
			if latestPod.Annotations[revisionKeys.UpdateRevisionKey] != targetRevision ||
				latestPod.Annotations[revisionKeys.UpdateCustomVersionKey] != targetCustomVersion {

				latestPod.Annotations[revisionKeys.UpdateRevisionKey] = targetRevision
				latestPod.Annotations[revisionKeys.UpdateCustomVersionKey] = targetCustomVersion

				klog.Infof("Updating TargetRevision for Pod %s/%s to %s to trigger reload-sidecar update", latestPod.Namespace, latestPod.Name, targetRevision)
				err := r.Update(ctx, latestPod)
				if err == nil {
					requeue = true
				}
				return err
			}

			// 2. Execute EffectPolicy logic
			if cms.Spec.EffectPolicy != nil {
				switch cms.Spec.EffectPolicy.Type {
				case appsv1alpha1.ReStartEffectPolicyType:
					if latestPod.Labels == nil {
						latestPod.Labels = make(map[string]string)
					}
					podNameForHash := latestPod.Name
					if podNameForHash == "" {
						podNameForHash = "unnamed"
					}
					hashBytes := md5.Sum([]byte(podNameForHash + targetRevision))
					hashStr := hex.EncodeToString(hashBytes[:])

					reloadSidecarName := r.getReloadSidecarName(ctx, cms)
					needRestart := false
					restartKey := GetConfigMapSetReloadSidecarRestartKey(cms.Name)

					if latestPod.Annotations[restartKey] != hashStr {
						latestPod.Annotations[restartKey] = hashStr
						needRestart = true
					}

					if needRestart {
						klog.Infof("Triggering reload-sidecar container inplace-update for Pod %s/%s to revision %s", latestPod.Namespace, latestPod.Name, targetRevision)
						err = r.Update(ctx, latestPod)
						if err != nil {
							requeue = true
							return err
						}
						err = r.rebootSidecarByCrr(latestPod, reloadSidecarName, hashStr)
						if err == nil {
							requeue = true
						}
						return err
					}

					// wait reload-sidecar reboot success
					err = r.waitSidecarRebootByCrrSuccess(latestPod, reloadSidecarName, hashStr)
					if err != nil {
						klog.Infof("Pod %s/%s reload-sidecar (%s) is not rebooted yet, waiting...", latestPod.Namespace, latestPod.Name, reloadSidecarName)
						requeue = true
						return nil
					}

					// wait reload-sidecar is ready
					isReloadSidecarReady := false
					for _, cs := range latestPod.Status.ContainerStatuses {
						if cs.Name == reloadSidecarName {
							if cs.Ready {
								isReloadSidecarReady = true
							}
							break
						}
					}
					if !isReloadSidecarReady {
						klog.Infof("Pod %s/%s reload-sidecar (%s) is not ready yet, waiting...", latestPod.Namespace, latestPod.Name, reloadSidecarName)
						requeue = true
						return nil
					}

					if err = r.verifySharedVolumeUpdated(ctx, latestPod, cms); err != nil {
						klog.Infof("Pod %s/%s verify shared volume failed: %v, waiting...", latestPod.Namespace, latestPod.Name, err)
						requeue = true
						return nil
					}

					rebootContainerNames := []string{}
					for _, c := range cms.Spec.Containers {
						cName := GetContainerName(latestPod, c)
						if cName == "" {
							klog.Warningf("Pod %s/%s cannot determine container name from spec %v", latestPod.Namespace, latestPod.Name, c)
							continue
						}
						annotationKey := GetConfigMapSetContainerRestartKey(cms.Name, cName)
						if latestPod.Annotations[annotationKey] != hashStr {
							latestPod.Annotations[annotationKey] = hashStr
							rebootContainerNames = append(rebootContainerNames, cName)
							needRestart = true
						}
					}

					if needRestart {
						klog.Infof("Triggering business container inplace-update for Pod %s/%s to revision %s", latestPod.Namespace, latestPod.Name, targetRevision)
						err = r.Update(ctx, latestPod)
						if err != nil {
							return err
						}
						err = r.rebootSidecarsByCrr(latestPod, rebootContainerNames, hashStr)
						if err == nil {
							requeue = true
						}
						return err
					}

					// wait business-sidecar reboot success
					err = r.waitSidecarsRebootByCrrSuccess(latestPod, rebootContainerNames, hashStr)
					if err != nil {
						klog.Infof("Pod %s/%s business-sidecars (%v) is not rebooted yet, waiting...", latestPod.Namespace, latestPod.Name, rebootContainerNames)
						requeue = true
						return nil
					}
				case appsv1alpha1.HotUpdateEffectPolicyType:
					// wait reload-sidecar is ready
					isReloadSidecarReady := false
					expectedSidecarName := r.getReloadSidecarName(ctx, cms)
					for _, cs := range latestPod.Status.ContainerStatuses {
						if cs.Name == expectedSidecarName {
							if cs.Ready {
								isReloadSidecarReady = true
							}
							break
						}
					}
					if !isReloadSidecarReady {
						klog.Infof("Pod %s/%s reload-sidecar (%s) is not ready yet, waiting...", latestPod.Namespace, latestPod.Name, expectedSidecarName)
						requeue = true
						return nil
					}

					if err = r.verifySharedVolumeUpdated(ctx, latestPod, cms); err != nil {
						klog.Infof("Pod %s/%s verify shared volume failed: %v, waiting...", latestPod.Namespace, latestPod.Name, err)
						requeue = true
						return nil
					}
				case appsv1alpha1.PostHookEffectPolicyType:
					if latestPod.Labels == nil {
						latestPod.Labels = make(map[string]string)
					}

					podNameForHash := latestPod.Name
					if podNameForHash == "" {
						podNameForHash = "unnamed"
					}
					hashBytes := md5.Sum([]byte(podNameForHash + targetRevision))
					hashStr := hex.EncodeToString(hashBytes[:])

					reloadSidecarName := r.getReloadSidecarName(ctx, cms)
					needRestart := false
					restartKey := GetConfigMapSetReloadSidecarRestartKey(cms.Name)
					if latestPod.Annotations[restartKey] != hashStr {
						latestPod.Annotations[restartKey] = hashStr
						needRestart = true
					}

					if needRestart {
						klog.Infof("Triggering reload-sidecar container inplace-update for Pod %s/%s to revision %s", latestPod.Namespace, latestPod.Name, targetRevision)
						err = r.Update(ctx, latestPod)
						if err != nil {
							return err
						}
						err = r.rebootSidecarByCrr(latestPod, reloadSidecarName, hashStr)
						if err != nil {
							requeue = true
						}
						return err
					}

					// wait business-sidecar reboot success
					err = r.waitSidecarRebootByCrrSuccess(latestPod, reloadSidecarName, hashStr)
					if err != nil {
						klog.Infof("Pod %s/%s reload-sidecar (%s) is not rebooted yet, waiting...", latestPod.Namespace, latestPod.Name, reloadSidecarName)
						requeue = true
						return nil
					}

					// wait reload-sidecar is ready
					isReloadSidecarReady := false
					for _, cs := range latestPod.Status.ContainerStatuses {
						if cs.Name == reloadSidecarName {
							if cs.Ready {
								isReloadSidecarReady = true
							}
							break
						}
					}
					if !isReloadSidecarReady {
						klog.Infof("Pod %s/%s reload-sidecar (%s) is not ready yet, waiting...", latestPod.Namespace, latestPod.Name, reloadSidecarName)
						requeue = true
						return nil
					}

					if err = r.verifySharedVolumeUpdated(ctx, latestPod, cms); err != nil {
						klog.Infof("Pod %s/%s verify shared volume failed: %v, waiting...", latestPod.Namespace, latestPod.Name, err)
						requeue = true
						return nil
					}

					if err = r.executePostHook(ctx, latestPod, cms); err != nil {
						klog.Infof("Pod %s/%s PostHook execution failed: %v, waiting...", latestPod.Namespace, latestPod.Name, err)
						requeue = true
						return nil
					}
				}
			}

			// Update CurrentRevisionKey and CurrentCustomVersionKey
			latestPod.Annotations[revisionKeys.CurrentRevisionKey] = targetRevision
			latestPod.Annotations[revisionKeys.CurrentCustomVersionKey] = targetCustomVersion

			klog.Infof("Successfully executed EffectPolicy and updated CurrentRevision for Pod %s/%s to %s", latestPod.Namespace, latestPod.Name, targetRevision)
			return r.Update(ctx, latestPod)
		})

		if err != nil {
			klog.Errorf("Failed to update pod annotations for %s/%s: %v", pod.Namespace, pod.Name, err)
			return requeue, err
		}
	}

	return requeue, nil
}

// updateStatus 更新cms的status字段
func (r *ReconcileConfigMapSet) updateStatus(ctx context.Context, request reconcile.Request, cms *appsv1alpha1.ConfigMapSet) error {
	klog.Infof("Updating status for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	// 获取关联Pod列表
	pods, err := GetMatchedPods(ctx, r.Client, cms)
	if err != nil {
		return err
	}
	var updateRevision string
	var updatedPodsNum, readyPodsNum, updatedReadyPodsNum int32
	// apps.kruise.io/configmapset.configMapSet名称.revision
	targetRevisionKey := GetConfigMapSetUpdateRevisionKey(cms.Name)
	currentRevisionKey := GetConfigMapSetCurrentRevisionKey(cms.Name)

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
		cms.Status.ReadyReplicas == readyPodsNum &&
		cms.Status.Replicas == int32(len(pods)) &&
		cms.Status.UpdatedReplicas == updatedPodsNum &&
		cms.Status.UpdatedReadyReplicas == updatedReadyPodsNum {
		klog.Infof("No change in status for ConfigMapSet %s/%s, skipping update", cms.Namespace, cms.Name)
		return nil
	}

	// 更新ConfigMapSet状态
	// 所有pod都是最新版以后才能更新currentRevision
	// 如果cms是第一次发版, 就使用当前版本
	if updatedReadyPodsNum == int32(len(pods)) || cms.Status.CurrentRevision == "" {
		cms.Status.CurrentRevision = updateRevision
		cms.Status.CurrentCustomVersion = cms.Spec.CustomVersion
		// 仅在所有pod都更新到一个版本以后进行版本维护
		err = r.cleanHistoryRevision(ctx, cms)
	}
	cms.Status.ObservedGeneration = cms.Generation
	cms.Status.UpdateRevision = updateRevision
	cms.Status.UpdateCustomVersion = cms.Spec.CustomVersion
	cms.Status.Replicas = int32(len(pods))
	cms.Status.ReadyReplicas = readyPodsNum
	cms.Status.UpdatedReplicas = updatedPodsNum
	cms.Status.UpdatedReadyReplicas = updatedReadyPodsNum
	// 根据更新策略计算 ExpectedUpdatedReplicas
	expected := int32(len(pods))
	if cms.Spec.UpdateStrategy.Partition != nil {
		partitionStr := cms.Spec.UpdateStrategy.Partition.String()
		if strings.HasSuffix(partitionStr, "%") {
			percentStr := strings.TrimSuffix(partitionStr, "%")
			if percent, err := strconv.Atoi(percentStr); err == nil && percent >= 0 && percent <= 100 {
				expected = int32(len(pods)) - int32(math.Ceil(float64(percent)/100.0*float64(len(pods))))
			}
		} else if count, err := strconv.Atoi(partitionStr); err == nil && count >= 0 {
			expected = int32(len(pods)) - int32(count)
		}
		if expected < 0 {
			expected = 0
		}
	}
	cms.Status.ExpectedUpdatedReplicas = expected

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

		klog.Info("Before update, RV:", latest.ResourceVersion)

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

func GetContainerName(pod *corev1.Pod, containerSpec appsv1alpha1.ContainerInjectSpec) string {
	if containerSpec.Name != "" {
		return containerSpec.Name
	}
	if containerSpec.NameFrom != nil && containerSpec.NameFrom.FieldRef.FieldPath != "" {
		path, subscript, ok := fieldpath.SplitMaybeSubscriptedPath(containerSpec.NameFrom.FieldRef.FieldPath)
		if ok {
			switch path {
			case "metadata.annotations":
				if pod.Annotations != nil {
					return pod.Annotations[subscript]
				}
			case "metadata.labels":
				if pod.Labels != nil {
					return pod.Labels[subscript]
				}
			}
		}
	}
	return ""
}

func (r *ReconcileConfigMapSet) rebootSidecarByCrr(pod *corev1.Pod, containerName string, hashStr string) error {
	return r.rebootSidecarsByCrr(pod, []string{containerName}, hashStr)
}

func (r *ReconcileConfigMapSet) rebootSidecarsByCrr(pod *corev1.Pod, containerNames []string, hashStr string) error {
	if len(containerNames) == 0 {
		return nil
	}

	// Sort containerNames to make sure the CRR has deterministic containers order
	sortedContainerNames := make([]string, len(containerNames))
	copy(sortedContainerNames, containerNames)
	sort.Strings(sortedContainerNames)

	// Combine hashStr and sortedContainerNames to form a unique hash for this specific set of containers and targetRevision
	combinedHashStr := hex.EncodeToString(md5.New().Sum([]byte(hashStr + "-" + strings.Join(sortedContainerNames, ","))))[:10]
	crrName := fmt.Sprintf("%s-%s", pod.Name, combinedHashStr)

	// Check if CRR already exists first
	existingCrr := &appsv1alpha1.ContainerRecreateRequest{}
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: crrName}, existingCrr)
	if err == nil {
		klog.Infof("CRR %s already exists for pod %s/%s to reboot containers %v", crrName, pod.Namespace, pod.Name, containerNames)
		return nil
	} else if !errors.IsNotFound(err) {
		klog.Errorf("Failed to get CRR %s for pod %s/%s: %v", crrName, pod.Namespace, pod.Name, err)
		return err
	}

	// Create a new CRR
	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crrName,
			Namespace: pod.Namespace,
			Labels: map[string]string{
				GetConfigMapSetCrrKey(): "true",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: pod.Name,
			Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
				FailurePolicy:   appsv1alpha1.ContainerRecreateRequestFailurePolicyIgnore,
				OrderedRecreate: true,
			},
			ActiveDeadlineSeconds:   pointer.Int64(300),
			TTLSecondsAfterFinished: pointer.Int32(10),
		},
	}

	for _, name := range sortedContainerNames {
		crr.Spec.Containers = append(crr.Spec.Containers, appsv1alpha1.ContainerRecreateRequestContainer{
			Name: name,
		})
	}

	err = r.Create(context.TODO(), crr)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		klog.Errorf("Failed to create CRR for pod %s/%s containers %v: %v", pod.Namespace, pod.Name, containerNames, err)
		return err
	}

	klog.Infof("Created CRR %s for pod %s/%s to reboot containers %v", crr.Name, pod.Namespace, pod.Name, containerNames)
	return nil
}

func (r *ReconcileConfigMapSet) waitSidecarRebootByCrrSuccess(pod *corev1.Pod, containerName string, hashStr string) error {
	return r.waitSidecarsRebootByCrrSuccess(pod, []string{containerName}, hashStr)
}

func (r *ReconcileConfigMapSet) waitSidecarsRebootByCrrSuccess(pod *corev1.Pod, containerNames []string, hashStr string) error {
	if len(containerNames) == 0 {
		return nil
	}

	// Sort containerNames to make sure the CRR has deterministic containers order
	sortedContainerNames := make([]string, len(containerNames))
	copy(sortedContainerNames, containerNames)
	sort.Strings(sortedContainerNames)

	// Combine hashStr and sortedContainerNames to form a unique hash for this specific set of containers and targetRevision
	combinedHashStr := hex.EncodeToString(md5.New().Sum([]byte(hashStr + "-" + strings.Join(sortedContainerNames, ","))))[:10]
	crrName := fmt.Sprintf("%s-%s", pod.Name, combinedHashStr)

	crr := &appsv1alpha1.ContainerRecreateRequest{}
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: crrName}, crr)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get CRR %s: %v", crrName, err)
	}

	if crr.Status.Phase != appsv1alpha1.ContainerRecreateRequestCompleted {
		return fmt.Errorf("CRR %s is in phase %s", crrName, crr.Status.Phase)
	}

	// For Completed CRR, verify that all target containers were recreated successfully
	for _, cState := range crr.Status.ContainerRecreateStates {
		if cState.Phase != appsv1alpha1.ContainerRecreateRequestSucceeded {
			return fmt.Errorf("container %s in CRR %s is in phase %s, message: %s", cState.Name, crrName, cState.Phase, cState.Message)
		}
	}

	// Clean up the CRR since we have successfully rebooted
	err = r.Delete(context.TODO(), crr)
	if err != nil && !errors.IsNotFound(err) {
		klog.Errorf("Failed to delete completed CRR %s for pod %s/%s: %v", crrName, pod.Namespace, pod.Name, err)
		return err
	} else {
		klog.Infof("Successfully deleted completed CRR %s for pod %s/%s", crrName, pod.Namespace, pod.Name)
	}

	return nil
}
