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
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/fieldpath"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runController "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	CustomVersion        string                               `json:"customVersion,omitempty"`
	Selector             *metav1.LabelSelector                `json:"selector"`
	Data                 map[string]string                    `json:"data"`
	Containers           []appsv1alpha1.ConfigMapSetContainer `json:"containers"`
	ReloadSidecarConfig  *appsv1alpha1.ReloadSidecarConfig    `json:"reloadSidecarConfig,omitempty"`
	EffectPolicy         *appsv1alpha1.EffectPolicy           `json:"effectPolicy,omitempty"`
	RevisionHistoryLimit *int32                               `json:"revisionHistoryLimit,omitempty"`
}

// UpdateInfo stores the result of pod update calculation
type UpdateInfo struct {
	PodsToUpdate         []*corev1.Pod
	TargetRevisions      []string
	TargetCustomVersions []string
}

const (
	ConfigMapFinalizerName = "finalizer.configmapset.kruise.io"
)

// RevisionEntry defines the data structure for storing revisions
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

	// Add Finalizer (if not present)
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

	// Sync Pods
	requeue, err := r.SyncPods(ctx, cms, pods)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("syncPods failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	if requeue {
		klog.Infof("ConfigMapSet %s/%s needs requeue to wait for pod updates", cms.Namespace, cms.Name)
		return reconcile.Result{RequeueAfter: 3 * time.Second}, nil
	}

	// Update status
	err = r.updateStatus(ctx, request, cms)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("updateStatus failed for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileConfigMapSet) cleanupConfigMap(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	cmName := GetConfigMapSetHubName(cms.Name)
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

// CalculateHash computes the hash of an object
func CalculateHash(v interface{}) (string, error) {
	if v == nil {
		return "", fmt.Errorf("object is nil")
	}
	// 1. Marshal spec to JSON
	specBytes, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec: %v", err)
	}

	// 2. Compute SHA256 hash
	hash := sha256.Sum256(specBytes)

	// 3. Return hexadecimal encoded hash string (first 8 bytes for short hash)
	return hex.EncodeToString(hash[:8]), nil
}

// syncRevisions manages version history
func (r *ReconcileConfigMapSet) syncRevisions(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	hash, err := CalculateHash(cms.Spec.Data)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %v", err)
	}

	// ConfigMap name: cms.Name + "-hub"
	cmName := GetConfigMapSetHubName(cms.Name)
	cmNamespace := cms.Namespace
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm := &corev1.ConfigMap{}
		if err = r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
			if errors.IsNotFound(err) {
				newCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cmName,
						Namespace: cmNamespace,
					},
					Data: make(map[string]string),
				}
				if err = controllerutil.SetControllerReference(cms, newCM, r.scheme); err != nil {
					return fmt.Errorf("failed to set owner reference on ConfigMap %s/%s: %v", cmNamespace, cmName, err)
				}
				if err = r.Create(ctx, newCM); err != nil {
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
		cm = cm.DeepCopy() // Avoid modifying the cached object
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data["revisions"] = string(revBytes)

		// Update ConfigMap
		return r.Update(ctx, cm)
	})
}

func (r *ReconcileConfigMapSet) getHubRevisions(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) ([]RevisionEntry, error) {
	cmName := GetConfigMapSetHubName(cms.Name)
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cms.Namespace}, cm); err != nil {
		return nil, err
	}
	var revisions []RevisionEntry
	if revData, exists := cm.Data["revisions"]; exists {
		if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
			return nil, err
		}
	}
	return revisions, nil
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
	// Load the version list from the hub ConfigMap for Pod selection priority sorting
	hubRevisions, err := r.getHubRevisions(ctx, cms)
	if err != nil {
		klog.Errorf("Failed to load hub revisions for ConfigMapSet %s/%s: %v, pod selection priority rules 2&3 will be degraded", cms.Namespace, cms.Name, err)
		return false, err
	}

	// Select Pods to update based on the update strategy
	updateStrategy := cms.Spec.UpdateStrategy

	requeue := false

	podGroups := GroupPodsByMatchLabelKeys(pods, updateStrategy.MatchLabelKeys)

	var allErrs []error
	for _, group := range podGroups {
		distributions, err := getDistributionByPartition(cms, group)
		if err != nil {
			klog.Errorf("failed to transform partition to distribution: %v", err)
			allErrs = append(allErrs, fmt.Errorf("failed to transform partition to distribution: %v", err))
			continue
		}
		groupRequeue, err := r.UpdateByDistribution(ctx, cms, distributions, group, revisionKeys, hubRevisions)
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

// Distribution is used internally in controller now
type Distribution struct {
	Revision      string `json:"revision,omitempty"`
	CustomVersion string `json:"customVersion,omitempty"`
	Reserved      int32  `json:"reserved"`
}

func getDistributionByPartition(cms *appsv1alpha1.ConfigMapSet, pods []*corev1.Pod) ([]Distribution, error) {
	// Total replicas = number of passed pods
	replicas := len(pods)
	// Parse partition
	partition := cms.Spec.UpdateStrategy.Partition

	// Calculate the latest updateRevision here since syncPods is called before updateStatus
	updateRevision, err := CalculateHash(cms.Spec.Data) // Calculate current version
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}
	updateCustomVersion := cms.Spec.CustomVersion
	currentRevision := cms.Status.CurrentRevision
	currentCustomVersion := cms.Status.CurrentCustomVersion

	newReplicas := replicas // New replicas count
	if partition != nil {
		partitionStr := partition.String()
		if strings.HasSuffix(partitionStr, "%") {
			// Handle percentage partition
			percentStr := strings.TrimSuffix(partitionStr, "%")
			percent, err := strconv.ParseInt(percentStr, 10, 32)
			if err != nil || percent < 0 || percent > 100 {
				return nil, fmt.Errorf("invalid partition percentage: %s", partitionStr)
			}
			newReplicas = replicas - int(math.Ceil(float64(percent)/100.0*float64(replicas)))
		} else if count, err := strconv.ParseInt(partitionStr, 10, 32); err == nil && count >= 0 {
			// Handle integer partition
			newReplicas = replicas - int(count)
			if newReplicas < 0 { // Cannot be negative
				newReplicas = 0
			}
		} else {
			return nil, fmt.Errorf("invalid partition number: %s", partitionStr)
		}
	}
	// Construct distribution
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

func getUpdateInfoByDistribution(cms *appsv1alpha1.ConfigMapSet, distributions []Distribution, pods []*corev1.Pod, revisionKeys *RevisionKeys, hubRevisions []RevisionEntry) (*UpdateInfo, error) {
	// Collect pods to update (excess version -> target version)
	var podsToUpdate []*corev1.Pod
	targetRevisions := make([]string, 0)
	targetCustomVersions := make([]string, 0)

	podsToUpdate, targetRevisions, targetCustomVersions = getUpdatePodsByDistributions(cms, distributions, pods, revisionKeys, hubRevisions)
	return &UpdateInfo{
		PodsToUpdate:         podsToUpdate,
		TargetRevisions:      targetRevisions,
		TargetCustomVersions: targetCustomVersions,
	}, nil
}

func getUpdatePodsByDistributions(cms *appsv1alpha1.ConfigMapSet, distributions []Distribution, pods []*v1.Pod, revisionKeys *RevisionKeys, hubRevisions []RevisionEntry) ([]*v1.Pod, []string, []string) {
	updateDistribution := distributions[0]
	currentDistribution := distributions[1]

	var validPods []*v1.Pod
	var podsToUpdate []*v1.Pod
	var targetRevisions []string
	var targetCustomVersions []string

	// Filter pods that don't have the enabled annotation and count unavailable pods
	var currentUnavailableCount int32 = 0
	for _, pod := range pods {
		if pod.Annotations != nil && pod.Annotations[GetConfigMapSetEnabledKey()] == "true" {
			validPods = append(validPods, pod)
			if !IsPodReady(pod) {
				currentUnavailableCount++
			}
		}
	}

	// Calculate maxUnavailable quota
	var maxUnavailableQuota int32 = math.MaxInt32 // Default to unlimited if not set
	if cms.Spec.UpdateStrategy.MaxUnavailable != nil {
		totalValidPods := len(validPods)
		maxUnavailableStr := cms.Spec.UpdateStrategy.MaxUnavailable.String()
		if strings.HasSuffix(maxUnavailableStr, "%") {
			percentStr := strings.TrimSuffix(maxUnavailableStr, "%")
			if percent, err := strconv.ParseInt(percentStr, 10, 32); err == nil && percent >= 0 && percent <= 100 {
				maxUnavailableQuota = int32(math.Ceil(float64(percent) / 100.0 * float64(totalValidPods)))
			}
		} else if count, err := strconv.ParseInt(maxUnavailableStr, 10, 32); err == nil && count >= 0 {
			maxUnavailableQuota = int32(count)
		}
	}

	// The actual quota allowed for this reconcile round
	allowedUpdateQuota := maxUnavailableQuota - currentUnavailableCount
	if allowedUpdateQuota <= 0 && maxUnavailableQuota != math.MaxInt32 {
		klog.Infof("ConfigMapSet %s/%s maxUnavailable limit reached (current unavailable: %d, max allowed: %d), no more pods will be updated in this round", cms.Namespace, cms.Name, currentUnavailableCount, maxUnavailableQuota)
		return podsToUpdate, targetRevisions, targetCustomVersions
	}

	// Calculate how many pods are currently at the update revision
	var currentUpdateRevisionCount int32 = 0
	for _, pod := range pods {
		isUpdated := false
		if pod.Annotations == nil {
			isUpdated = true
		} else {
			hasCurrentRev := pod.Annotations[revisionKeys.CurrentRevisionKey] != ""
			hasUpdateRev := pod.Annotations[revisionKeys.UpdateRevisionKey] != ""
			// If it does not have currentVersion and UpdateVersion annotations, we treat it as updated
			if !hasCurrentRev && !hasUpdateRev {
				isUpdated = true
			} else if pod.Annotations[revisionKeys.CurrentRevisionKey] == updateDistribution.Revision {
				isUpdated = true
			}
		}
		if isUpdated {
			currentUpdateRevisionCount++
		}
	}

	// Calculate how many pods we need to update
	needToUpdate := updateDistribution.Reserved - currentUpdateRevisionCount
	if needToUpdate <= 0 {
		return podsToUpdate, targetRevisions, targetCustomVersions
	}

	// Build hub revision index: hash -> position in hub (smaller is older)
	hubRevisionIndex := make(map[string]int, len(hubRevisions))
	for idx, rev := range hubRevisions {
		hubRevisionIndex[rev.Hash] = idx
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

		// For non-currentRevision pods, apply rules 2 and 3
		if !isICurrent && !isJCurrent {
			idxI, inRMCI := hubRevisionIndex[revI]
			idxJ, inRMCJ := hubRevisionIndex[revJ]

			// 3. Not in RMC > In RMC
			if !inRMCI && inRMCJ {
				return true
			}
			if inRMCI && !inRMCJ {
				return false
			}

			// 2. Smaller index (older) > Larger index (newer)
			if inRMCI && inRMCJ && idxI != idxJ {
				return idxI < idxJ
			}
		}

		if podI.CreationTimestamp.Equal(&podJ.CreationTimestamp) {
			return podI.Name < podJ.Name
		}

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

func (r *ReconcileConfigMapSet) getReloadSidecarName(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) string {
	expectedSidecarName := GetConfigMapSetDefaultSidecarName(cms.Name)
	if cms.Spec.ReloadSidecarConfig != nil {
		if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.ReloadSidecarTypeK8s && cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.Name != "" {
			expectedSidecarName = cms.Spec.ReloadSidecarConfig.Config.Name
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.ReloadSidecarTypeSidecarSet && cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.SidecarSetRef != nil {
			expectedSidecarName = cms.Spec.ReloadSidecarConfig.Config.SidecarSetRef.ContainerName
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.ReloadSidecarTypeCustom {
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

// UpdateByDistribution updates pods based on the distribution
func (r *ReconcileConfigMapSet) UpdateByDistribution(ctx context.Context, cms *appsv1alpha1.ConfigMapSet, distributions []Distribution, pods []*corev1.Pod, revisionKeys *RevisionKeys, hubRevisions []RevisionEntry) (bool, error) {
	// Step 1: Calculate the current version distribution and the update version queue
	updateInfo, err := getUpdateInfoByDistribution(cms, distributions, pods, revisionKeys, hubRevisions)
	if err != nil {
		klog.Errorf("failed to fetch update info: %v", err)
		return false, err
	}

	// Step 2: Trigger updates
	requeue := false
	for i, pod := range updateInfo.PodsToUpdate {
		if i >= len(updateInfo.TargetRevisions) || i >= len(updateInfo.TargetCustomVersions) {
			break
		}
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
				err = r.Update(ctx, latestPod)
				if err == nil {
					requeue = true
				}
				return err
			}

			// 2. Execute EffectPolicy logic
			if cms.Spec.EffectPolicy != nil {
				switch cms.Spec.EffectPolicy.Type {
				case appsv1alpha1.EffectPolicyTypeReStart:
					if latestPod.Annotations == nil {
						latestPod.Annotations = make(map[string]string)
					}

					reloadSidecarName := r.getReloadSidecarName(ctx, cms)
					needRestart := false
					reloadSidecarRestartKey := GetConfigMapSetReloadSidecarRestartKey(cms.Name)
					expectHash := GetContainerHash(latestPod, targetRevision)
					if latestPod.Annotations[reloadSidecarRestartKey] != expectHash {
						latestPod.Annotations[reloadSidecarRestartKey] = expectHash
						needRestart = true
					}

					if needRestart {
						klog.Infof("Triggering reload-sidecar container inplace-update for Pod %s/%s to revision %s", latestPod.Namespace, latestPod.Name, targetRevision)
						for _, status := range latestPod.Status.ContainerStatuses {
							if status.Name == reloadSidecarName && status.State.Running != nil {
								latestPod.Annotations[GetConfigMapSetContainerStartedAtKey(reloadSidecarName)] = status.State.Running.StartedAt.Format(time.RFC3339)
							}
						}
						err = r.rebootSidecarByCrr(latestPod, reloadSidecarName, expectHash)
						if err != nil {
							return err
						}
						err = r.Update(ctx, latestPod)
						if err == nil {
							requeue = true
						}
						return err
					}

					// wait reload-sidecar reboot success
					err = r.waitSidecarRebootByCrrSuccess(ctx, latestPod, reloadSidecarName, expectHash)
					if err != nil {
						klog.Errorf("Pod %s/%s reload-sidecar (%s) is not rebooted yet because of %s, waiting...", latestPod.Namespace, latestPod.Name, reloadSidecarName, err.Error())
						return err
					}

					time.Sleep(time.Second * 3)

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

					allContainerNames := []string{}
					rebootContainerNames := []string{}
					for _, c := range cms.Spec.Containers {
						cName := GetContainerName(latestPod, c)
						if cName == "" {
							klog.Warningf("Pod %s/%s cannot determine container name from spec %v", latestPod.Namespace, latestPod.Name, c)
							continue
						}
						allContainerNames = append(allContainerNames, cName)
						annotationKey := GetConfigMapSetContainerRestartKey(cms.Name, cName)
						if latestPod.Annotations[annotationKey] != expectHash {
							latestPod.Annotations[annotationKey] = expectHash
							rebootContainerNames = append(rebootContainerNames, cName)
							needRestart = true
						}
					}

					if needRestart {
						klog.Infof("Triggering business container inplace-update for Pod %s/%s to revision %s", latestPod.Namespace, latestPod.Name, targetRevision)
						for _, name := range rebootContainerNames {
							for _, status := range latestPod.Status.ContainerStatuses {
								if status.Name == name && status.State.Running != nil {
									latestPod.Annotations[GetConfigMapSetContainerStartedAtKey(name)] = status.State.Running.StartedAt.Format(time.RFC3339)
								}
							}
							err = r.rebootSidecarByCrr(latestPod, name, expectHash)
							if err != nil {
								return err
							}
						}

						err = r.Update(ctx, latestPod)
						if err == nil {
							requeue = true
						}
						return err
					}

					// wait business-sidecar reboot success
					for _, containerName := range allContainerNames {
						err = r.waitSidecarRebootByCrrSuccess(ctx, latestPod, containerName, expectHash)
						if err != nil {
							klog.Infof("Pod %s/%s business-sidecars (%v) is not rebooted yet, waiting...", latestPod.Namespace, latestPod.Name, rebootContainerNames)
							requeue = true
							return err
						}
					}
				case appsv1alpha1.EffectPolicyTypeHotUpdate:
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
				case appsv1alpha1.EffectPolicyTypePostHook:
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
						for _, status := range latestPod.Status.ContainerStatuses {
							if status.Name == reloadSidecarName && status.State.Running != nil {
								latestPod.Annotations[GetConfigMapSetContainerStartedAtKey(reloadSidecarName)] = status.State.Running.StartedAt.Format(time.RFC3339)
							}
						}
						err = r.rebootSidecarByCrr(latestPod, reloadSidecarName, hashStr)
						if err != nil {
							return err
						}
						err = r.Update(ctx, latestPod)
						if err == nil {
							requeue = true
						}
						return err
					}

					// wait business-sidecar reboot success
					err = r.waitSidecarRebootByCrrSuccess(ctx, latestPod, reloadSidecarName, hashStr)
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

// updateStatus updates the status field of cms
func (r *ReconcileConfigMapSet) updateStatus(ctx context.Context, request reconcile.Request, cms *appsv1alpha1.ConfigMapSet) error {
	klog.Infof("Updating status for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	// Get matched Pod list
	pods, err := GetMatchedPods(ctx, r.Client, cms)
	if err != nil {
		return err
	}
	var updateRevision string
	var updatedPodsNum, readyPodsNum, updatedReadyPodsNum int32
	// apps.kruise.io/configmapset.configMapSetName.revision
	targetRevisionKey := GetConfigMapSetUpdateRevisionKey(cms.Name)
	currentRevisionKey := GetConfigMapSetCurrentRevisionKey(cms.Name)

	updateRevision, err = CalculateHash(cms.Spec.Data) // Calculate current version
	if err != nil {
		return fmt.Errorf("failed to compute hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	// Calculate status metrics
	for _, pod := range pods {
		isUpdated := false
		if pod.Annotations == nil {
			isUpdated = true
		} else {
			hasCurrentRev := pod.Annotations[currentRevisionKey] != ""
			hasUpdateRev := pod.Annotations[targetRevisionKey] != ""
			if !hasCurrentRev && !hasUpdateRev {
				isUpdated = true
			} else if pod.Annotations[targetRevisionKey] == updateRevision && pod.Annotations[currentRevisionKey] == updateRevision {
				isUpdated = true
			}
		}

		if isUpdated {
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

	// Update ConfigMapSet status
	// currentRevision can only be updated after all pods are at the latest version
	// If it is the first release of cms, use the current version
	if updatedReadyPodsNum == int32(len(pods)) || cms.Status.CurrentRevision == "" {
		cms.Status.CurrentRevision = updateRevision
		cms.Status.CurrentCustomVersion = cms.Spec.CustomVersion
		// Manage version history only after all pods are updated to a version
		err = r.cleanHistoryRevision(ctx, cms)
	}
	cms.Status.ObservedGeneration = cms.Generation
	cms.Status.UpdateRevision = updateRevision
	cms.Status.UpdateCustomVersion = cms.Spec.CustomVersion
	cms.Status.Replicas = int32(len(pods))
	cms.Status.ReadyReplicas = readyPodsNum
	cms.Status.UpdatedReplicas = updatedPodsNum
	cms.Status.UpdatedReadyReplicas = updatedReadyPodsNum
	// Calculate ExpectedUpdatedReplicas based on update strategy
	expected := int32(len(pods))
	if cms.Spec.UpdateStrategy.Partition != nil {
		partitionStr := cms.Spec.UpdateStrategy.Partition.String()
		if strings.HasSuffix(partitionStr, "%") {
			percentStr := strings.TrimSuffix(partitionStr, "%")
			if percent, err := strconv.ParseInt(percentStr, 10, 32); err == nil && percent >= 0 && percent <= 100 {
				expected = int32(len(pods)) - int32(math.Ceil(float64(percent)/100.0*float64(len(pods))))
			}
		} else if count, err := strconv.ParseInt(partitionStr, 10, 32); err == nil && count >= 0 {
			expected = int32(len(pods)) - int32(count)
		}
		if expected < 0 {
			expected = 0
		}
	}
	cms.Status.ExpectedUpdatedReplicas = expected

	// Sync status to etcd
	klog.Infof("Updating cms %s status %#v", cms.Name, cms.Status)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the latest object
		latest := &appsv1alpha1.ConfigMapSet{}
		if err := r.Get(context.TODO(), request.NamespacedName, latest); err != nil {
			utilruntime.HandleError(fmt.Errorf("error getting latest configmapset %s/%s: %v", cms.Namespace, cms.Name, err))
			return err
		}

		// Update Status fields
		latest.Status = cms.Status

		klog.Info("Before update, RV:", latest.ResourceVersion)

		// Execute Status update
		if err := r.Status().Update(context.TODO(), latest); err != nil {
			utilruntime.HandleError(fmt.Errorf("error updating configmapset status %s/%s: %v", cms.Namespace, cms.Name, err))
			return err
		}

		// Log the updated ResourceVersion
		klog.Infof("Updated cms %s status successfully, new ResourceVersion: %s", latest.Name, latest.ResourceVersion)
		return nil
	})
}

func (r *ReconcileConfigMapSet) cleanHistoryRevision(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) error {
	cmName := GetConfigMapSetHubName(cms.Name)
	cmNamespace := cms.Namespace

	var revisions []RevisionEntry

	// Define update logic
	updateFunc := func() error {
		// Fetch the latest ConfigMap again to avoid concurrent conflicts
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
			if errors.IsNotFound(err) {
				// Create if ConfigMap does not exist
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

		// Parse existing revisions from ConfigMap
		if revData, exists := cm.Data["revisions"]; exists {
			if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
				klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
				return fmt.Errorf("failed to unmarshal revisions from ConfigMap %s: %v", cmName, err)
			}
		}

		// If the number of revisions reaches the limit, remove the oldest one that is not used by any Pod
		if cms.Spec.RevisionHistoryLimit != nil && int32(len(revisions)) >= *cms.Spec.RevisionHistoryLimit {
			pods, podErr := GetMatchedPods(ctx, r.Client, cms)
			if podErr != nil {
				return podErr
			}
			revisionsInUse := make(map[string]bool)
			currentRevisionKey := GetConfigMapSetCurrentRevisionKey(cms.Name)
			for _, pod := range pods {
				if pod.Annotations != nil && pod.Annotations[currentRevisionKey] != "" {
					revisionsInUse[pod.Annotations[currentRevisionKey]] = true
				}
			}
			// Find the oldest revision that is not in use
			oldestUnusedIdx := -1
			for i, rev := range revisions {
				if !revisionsInUse[rev.Hash] {
					oldestUnusedIdx = i
					break
				}
			}

			// If all revisions are in use, do not remove any
			if oldestUnusedIdx == -1 {
				klog.Warningf("All revisions are currently in use, skipping cleanup for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
				return nil
			}

			// Remove the oldest unused revision
			revToRemove := revisions[oldestUnusedIdx]
			klog.Infof("Removing oldest unused revision %s (customVersion=%s) from hub to maintain limit %d", revToRemove.Hash, revToRemove.CustomVersion, *cms.Spec.RevisionHistoryLimit)
			revisions = append(revisions[:oldestUnusedIdx], revisions[oldestUnusedIdx+1:]...)
		}

		klog.Infof("Updated revisions for ConfigMapSet %s/%s: %v", cms.Namespace, cms.Name, revisions)

		// Update ConfigMap Data
		revBytes, err := json.Marshal(revisions)
		if err != nil {
			return fmt.Errorf("failed to marshal revisions: %v", err)
		}
		cm = cm.DeepCopy() // Avoid modifying the cached object
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data["revisions"] = string(revBytes)

		// Update ConfigMap
		return r.Update(ctx, cm)
	}

	// Use `retry.RetryOnConflict` to handle update conflicts
	err := retry.RetryOnConflict(retry.DefaultRetry, updateFunc)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap %s: %v", cmName, err)
	}
	return nil
}

func GetContainerName(pod *corev1.Pod, containerSpec appsv1alpha1.ConfigMapSetContainer) string {
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

func (r *ReconcileConfigMapSet) genContainerRecreateRequestName(pod *corev1.Pod, containerName string, hashStr string) string {
	// Combine hashStr and sortedContainerNames to form a unique hash for this specific set of containers and targetRevision
	sum := md5.Sum([]byte(fmt.Sprintf("%s-%s", hashStr, containerName)))
	combinedHashStr := hex.EncodeToString(sum[:])[:10]
	return fmt.Sprintf("%s-%s", pod.Name, combinedHashStr)
}

func (r *ReconcileConfigMapSet) rebootSidecarByCrr(pod *corev1.Pod, containerName string, hashStr string) error {
	if len(containerName) == 0 {
		return nil
	}
	crrName := r.genContainerRecreateRequestName(pod, containerName, hashStr)
	// Check if CRR already exists first
	existingCrr := &appsv1alpha1.ContainerRecreateRequest{}
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: crrName}, existingCrr)
	if err == nil {
		klog.Infof("CRR %s already exists for pod %s/%s to reboot container %s", crrName, pod.Namespace, pod.Name, containerName)
		return nil
	} else if !errors.IsNotFound(err) {
		klog.Errorf("Failed to get CRR %s for pod %s/%s: %v", crrName, pod.Namespace, pod.Name, err)
		return err
	}

	// Create a new CRR
	isController := true
	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crrName,
			Namespace: pod.Namespace,
			Labels: map[string]string{
				GetConfigMapSetCrrKey(): "true",
			},
			Annotations: make(map[string]string),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       pod.Name,
					UID:        pod.UID,
					Controller: &isController,
				},
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: pod.Name,
			Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
				ForceRecreate:             true,
				FailurePolicy:             appsv1alpha1.ContainerRecreateRequestFailurePolicyFail,
				OrderedRecreate:           true,
				UnreadyGracePeriodSeconds: ptr.To[int64](5),
				MinStartedSeconds:         int32(3),
			},
			ActiveDeadlineSeconds:   pointer.Int64(14400),
			TTLSecondsAfterFinished: pointer.Int32(3),
		},
	}

	crr.Spec.Containers = []appsv1alpha1.ContainerRecreateRequestContainer{{Name: containerName}}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName && status.State.Running != nil {
			crr.Annotations[GetConfigMapSetContainerStartedAtKey(containerName)] = status.State.Running.StartedAt.Format(time.RFC3339)
		}
	}

	err = r.Create(context.TODO(), crr)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		klog.Errorf("Failed to create CRR for pod %s/%s container %s: %v", pod.Namespace, pod.Name, containerName, err)
		return err
	}

	klog.Infof("Created CRR %s for pod %s/%s to reboot container %s", crr.Name, pod.Namespace, pod.Name, containerName)
	return nil
}

func (r *ReconcileConfigMapSet) waitSidecarRebootByCrrSuccess(ctx context.Context, pod *corev1.Pod, containerName string, hashStr string) error {
	if len(containerName) == 0 {
		return nil
	}

	// Combine hashStr and sortedContainerNames to form a unique hash for this specific set of containers and targetRevision
	crrName := r.genContainerRecreateRequestName(pod, containerName, hashStr)

	crr := &appsv1alpha1.ContainerRecreateRequest{}
	err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: crrName}, crr)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("CRR %s not found in waitSidecarRebootByCrrSuccess", crrName)
			// Fallback to check if the pod's container has actually restarted
			recordedTimeStr := pod.Annotations[GetConfigMapSetContainerStartedAtKey(containerName)]
			if recordedTimeStr == "" {
				return fmt.Errorf("CRR %s not found startAt annotation", crrName)
			}
			recordedTime, parseErr := time.Parse(time.RFC3339, recordedTimeStr)
			if parseErr != nil {
				return parseErr
			}
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == containerName {
					if status.State.Running == nil {
						return fmt.Errorf("CRR %s not found and container %s is not running", crrName, containerName)
					}
					if !status.State.Running.StartedAt.After(recordedTime) {
						return fmt.Errorf("CRR %s not found and container %s has not restarted yet", crrName, containerName)
					}
				}
			}
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

	// Verify that the StartedAt time of all containers is strictly greater than the time recorded in the CRR
	recordedTimeStr := crr.Annotations[GetConfigMapSetContainerStartedAtKey(containerName)]
	if recordedTimeStr != "" {
		recordedTime, err := time.Parse(time.RFC3339, recordedTimeStr)
		if err == nil {
			found := false
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name == containerName {
					found = true
					if status.State.Running == nil {
						return fmt.Errorf("container %s is not running yet", containerName)
					}
					if !status.State.Running.StartedAt.After(recordedTime) {
						return fmt.Errorf("container %s has not been restarted yet (StartedAt <= recorded time)", containerName)
					}
				}
			}
			if !found {
				return fmt.Errorf("container %s not found in pod status", containerName)
			}
		}
	}

	// Clean up the CRR since we have successfully rebooted
	err = r.Delete(ctx, crr)
	if err != nil && !errors.IsNotFound(err) {
		klog.Errorf("Failed to delete completed CRR %s for pod %s/%s: %v", crrName, pod.Namespace, pod.Name, err)
		return err
	}

	return nil
}
