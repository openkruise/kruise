package sidecarset

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	podmutating "github.com/openkruise/kruise/pkg/webhook/default_server/pod/mutating"
	sidecarsetmutating "github.com/openkruise/kruise/pkg/webhook/default_server/sidecarset/mutating"
)

var (
	updateCache = &updatedPodCache{
		podSidecarUpdated: make(map[string]string),
	}
)

func isIgnoredPod(pod *corev1.Pod) bool {
	for _, namespace := range podmutating.SidecarIgnoredNamespaces {
		if pod.Namespace == namespace {
			return true
		}
	}
	return false
}

func calculateStatus(sidecarSet *appsv1alpha1.SidecarSet, pods []*corev1.Pod) (*appsv1alpha1.SidecarSetStatus, error) {
	var matchedPods, updatedPods, readyPods int32
	matchedPods = int32(len(pods))
	for _, pod := range pods {
		updated, err := isPodSidecarUpdated(sidecarSet, pod)
		if err != nil {
			return nil, err
		}
		if updated {
			updatedPods++
		}

		if isRunningAndReady(pod) {
			readyPods++
		}
	}

	return &appsv1alpha1.SidecarSetStatus{
		ObservedGeneration: sidecarSet.Generation,
		MatchedPods:        matchedPods,
		UpdatedPods:        updatedPods,
		ReadyPods:          readyPods,
	}, nil
}

func isPodSidecarUpdated(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) (bool, error) {
	hashKey := sidecarsetmutating.SidecarSetHashAnnotation
	if pod.Annotations[hashKey] == "" {
		return false, nil
	}

	sidecarSetHash := make(map[string]string)
	if err := json.Unmarshal([]byte(pod.Annotations[hashKey]), &sidecarSetHash); err != nil {
		return false, err
	}

	return sidecarSetHash[sidecarSet.Name] == sidecarSet.Annotations[hashKey], nil
}

func isRunningAndReady(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning && podutil.IsPodReady(pod)
}

func (r *ReconcileSidecarSet) updateSidecarSetStatus(sidecarSet *appsv1alpha1.SidecarSet, status *appsv1alpha1.SidecarSetStatus) error {
	if !inconsistentStatus(sidecarSet, status) {
		return nil
	}

	sidecarSetClone := sidecarSet.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		sidecarSetClone.Status = *status

		updateErr := r.Status().Update(context.TODO(), sidecarSetClone)
		if updateErr == nil {
			return nil
		}

		key := types.NamespacedName{
			Name: sidecarSetClone.Name,
		}

		if err := r.Get(context.TODO(), key, sidecarSetClone); err != nil {
			klog.Errorf("error getting updated sidecarset %s from client", sidecarSetClone.Name)
		}

		return updateErr
	})

	return err
}

func inconsistentStatus(sidecarSet *appsv1alpha1.SidecarSet, status *appsv1alpha1.SidecarSetStatus) bool {
	return status.ObservedGeneration > sidecarSet.Status.ObservedGeneration ||
		status.MatchedPods != sidecarSet.Status.MatchedPods ||
		status.UpdatedPods != sidecarSet.Status.UpdatedPods ||
		status.ReadyPods != sidecarSet.Status.ReadyPods
}

// add this cache to avoid be influenced by informer cache latency when controller try to count maxUnavailable
type updatedPodCache struct {
	lock sync.RWMutex
	// key is in the format: sidecarset name/pod namespace/pod name
	// value is sidecarset hash
	podSidecarUpdated map[string]string
}

func (u *updatedPodCache) set(key, hash string) {
	u.lock.Lock()
	u.podSidecarUpdated[key] = hash
	u.lock.Unlock()
}

func (u *updatedPodCache) delete(key string) {
	u.lock.Lock()
	delete(u.podSidecarUpdated, key)
	u.lock.Unlock()
}

// return true means: sidecar of pod is updated
func (u *updatedPodCache) isSidecarUpdated(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) bool {
	key := fmt.Sprintf("%v/%v/%v", sidecarSet.Name, pod.Namespace, pod.Name)
	u.lock.RLock()
	hash := u.podSidecarUpdated[key]
	u.lock.RUnlock()

	if hash == sidecarSet.Annotations[sidecarsetmutating.SidecarSetHashAnnotation] {
		return true
	}
	// if sidecarset changed, clean all stale cache related with this sidecarset
	if hash != "" {
		u.reset(sidecarSet)
	}
	return false
}

func (u *updatedPodCache) reset(sidecarSet *appsv1alpha1.SidecarSet) {
	prefix := fmt.Sprintf("%v/", sidecarSet.Name)
	u.lock.Lock()
	for key := range u.podSidecarUpdated {
		if strings.HasPrefix(key, prefix) {
			delete(u.podSidecarUpdated, key)
		}
	}
	u.lock.Unlock()
}

// available definition:
// 1. image in pod.spec and pod.status is exactly the same
// 2. pod is ready
func getUnavailableNumber(sidecarSet *appsv1alpha1.SidecarSet, pods []*corev1.Pod) (int, error) {
	var unavailableNum int
	for _, pod := range pods {
		// in case of informer cache latency
		key := fmt.Sprintf("%v/%v/%v", sidecarSet.Name, pod.Namespace, pod.Name)
		podInCacheUpdated := updateCache.isSidecarUpdated(sidecarSet, pod)
		podInInformerUpdated, err := isPodSidecarUpdated(sidecarSet, pod)
		if err != nil {
			return 0, err
		}
		if podInCacheUpdated && !podInInformerUpdated {
			unavailableNum++
			continue
		}
		if podInCacheUpdated && podInInformerUpdated {
			updateCache.delete(key)
		}

		if !isPodImageConsistent(pod) {
			unavailableNum++
			continue
		}

		if !isRunningAndReady(pod) {
			unavailableNum++
		}
	}
	return unavailableNum, nil
}

func isPodCreatedBeforeSidecarSet(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) (bool, error) {
	hashKey := sidecarsetmutating.SidecarSetHashAnnotation
	if pod.Annotations[hashKey] == "" {
		return true, nil
	}

	sidecarSetHash := make(map[string]string)
	if err := json.Unmarshal([]byte(pod.Annotations[hashKey]), &sidecarSetHash); err != nil {
		return false, err
	}
	if _, ok := sidecarSetHash[sidecarSet.Name]; !ok {
		return true, nil
	}
	return false, nil
}

// check if fields other than sidecar image had changed
func otherFieldsInSidecarChanged(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) (bool, error) {
	hashKey := sidecarsetmutating.SidecarSetHashWithoutImageAnnotation
	if pod.Annotations[hashKey] == "" {
		return false, nil
	}

	sidecarSetHash := make(map[string]string)
	if err := json.Unmarshal([]byte(pod.Annotations[hashKey]), &sidecarSetHash); err != nil {
		return false, err
	}

	return sidecarSetHash[sidecarSet.Name] != sidecarSet.Annotations[hashKey], nil
}

func isPodImageConsistent(pod *corev1.Pod) bool {
	containerSpecImage := make(map[string]string, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		containerSpecImage[container.Name] = container.Image
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerSpecImage[containerStatus.Name] != containerStatus.Image {
			return false
		}
	}
	return true
}

func (r *ReconcileSidecarSet) updateSidecarImageAndHash(sidecarSet *appsv1alpha1.SidecarSet, pods []*corev1.Pod) error {
	if len(pods) == 0 {
		return nil
	}

	// only support maxUnavailable=1 currently
	klog.V(3).Infof("try to update sidecar of %v/%v", pods[0].Namespace, pods[0].Name)
	if err := r.updatePodSidecarAndHash(sidecarSet, pods[0]); err != nil {
		return err
	}
	updateCache.set(
		fmt.Sprintf("%v/%v/%v", sidecarSet.Name, pods[0].Namespace, pods[0].Name),
		sidecarSet.Annotations[sidecarsetmutating.SidecarSetHashAnnotation])
	return nil
}

func (r *ReconcileSidecarSet) updatePodSidecarAndHash(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) error {
	podClone := pod.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// update sidecar image
		updatePodSidecar(sidecarSet, podClone)

		// update hash
		hashKey := sidecarsetmutating.SidecarSetHashAnnotation
		sidecarSetHash := make(map[string]string)
		if err := json.Unmarshal([]byte(podClone.Annotations[hashKey]), &sidecarSetHash); err != nil {
			return err
		}
		sidecarSetHash[sidecarSet.Name] = sidecarSet.Annotations[hashKey]
		newHash, err := json.Marshal(sidecarSetHash)
		if err != nil {
			return err
		}
		podClone.Annotations[hashKey] = string(newHash)

		updateErr := r.Update(context.TODO(), podClone)
		if updateErr == nil {
			return nil
		}

		key := types.NamespacedName{
			Namespace: podClone.Namespace,
			Name:      podClone.Name,
		}

		if err := r.Get(context.TODO(), key, podClone); err != nil {
			klog.Errorf("error getting updated pod %s from client", sidecarSet.Name)
		}

		return updateErr
	})

	return err
}

func updatePodSidecar(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) {
	sidecarImage := make(map[string]string, len(sidecarSet.Spec.Containers))
	for _, container := range sidecarSet.Spec.Containers {
		sidecarImage[container.Name] = container.Image
	}

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if image, ok := sidecarImage[container.Name]; ok {
			container.Image = image
		}
	}
}
