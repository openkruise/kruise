package livenessprobemapnodeprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	livenessprobeUtils "github.com/openkruise/kruise/pkg/util/livenessprobe"
)

func (r *ReconcileEnhancedLivenessProbeMapNodeProbe) delNodeProbeConfig(pod *v1.Pod) error {
	return r.retryOnConflictDelNodeProbeConfig(pod)
}

func (r *ReconcileEnhancedLivenessProbeMapNodeProbe) addNodeProbeConfig(pod *v1.Pod) error {
	plivenessProbeConfig, err := parseEnhancedLivenessProbeConfig(pod)
	if err != nil {
		return err
	}
	if pod.Spec.NodeName == "" {
		klog.Warningf("No found pod node name, pod: %v/%v", pod.Namespace, pod.Name)
		return nil
	}

	npp := appsv1alpha1.NodePodProbe{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, &npp)
	if err == nil {
		return r.retryOnConflictAddNodeProbeConfig(pod, plivenessProbeConfig)
	}
	if !errors.IsNotFound(err) {
		klog.Errorf("Failed to get node pod probe for pod: %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return err
	}
	newNppGenerated, err := r.generateNewNodePodProbe(pod)
	if err != nil {
		klog.Errorf("Failed to generate new node pod probe for pod: %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return err
	}
	if err = r.Create(context.TODO(), &newNppGenerated); err != nil {
		klog.Errorf("Failed to create node pod probe for pod: %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return err
	}
	return nil
}

func (r *ReconcileEnhancedLivenessProbeMapNodeProbe) retryOnConflictDelNodeProbeConfig(pod *v1.Pod) error {
	nppClone := &appsv1alpha1.NodePodProbe{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, nppClone); err != nil {
			if errors.IsNotFound(err) {
				klog.Warningf("No found getting updated npp %s from client", pod.Spec.NodeName)
				return nil
			}
			klog.Errorf("Error getting updated npp %s from client", pod.Spec.NodeName)
			return err
		}

		podNewNppCloneSpec := nppClone.Spec.DeepCopy()

		newPodProbeTmp := []appsv1alpha1.PodProbe{}
		for index := range podNewNppCloneSpec.PodProbes {
			podContainerProbe := podNewNppCloneSpec.PodProbes[index]
			if podContainerProbe.Name == pod.Name && podContainerProbe.Namespace == pod.Namespace &&
				podContainerProbe.UID == fmt.Sprintf("%v", pod.UID) {
				continue
			}
			newPodProbeTmp = append(newPodProbeTmp, podContainerProbe)
		}
		podNewNppCloneSpec.PodProbes = newPodProbeTmp

		// delete node pod probe
		if len(podNewNppCloneSpec.PodProbes) == 0 {
			if err := r.Delete(context.TODO(), nppClone); err != nil {
				klog.Errorf("Failed to delete node pod probe for pod: %s/%s, err: %v", pod.Namespace, pod.Name, err)
				return err
			}
			return nil
		}
		if reflect.DeepEqual(podNewNppCloneSpec, nppClone.Spec) {
			return nil
		}
		nppClone.Spec = *podNewNppCloneSpec
		return r.Client.Update(context.TODO(), nppClone)
	})
	if err != nil {
		klog.Errorf("NodePodProbe update NodePodProbe(%s) failed:%s", pod.Spec.NodeName, err.Error())
		return err
	}
	return nil
}

func (r *ReconcileEnhancedLivenessProbeMapNodeProbe) retryOnConflictAddNodeProbeConfig(pod *v1.Pod,
	containersLivenessProbeConfig []livenessprobeUtils.ContainerLivenessProbe) error {
	nppClone := &appsv1alpha1.NodePodProbe{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, nppClone); err != nil {
			if errors.IsNotFound(err) {
				klog.Warningf("No found getting updated npp %s from client", pod.Spec.NodeName)
				return nil
			}
			klog.Errorf("Error getting updated npp %s from client", pod.Spec.NodeName)
			return err
		}

		podNewNppCloneSpec := nppClone.Spec.DeepCopy()

		isHit := false
		isChanged := true
		for index := range podNewNppCloneSpec.PodProbes {
			podContainerProbe := &podNewNppCloneSpec.PodProbes[index]
			if podContainerProbe.Name == pod.Name && podContainerProbe.Namespace == pod.Namespace &&
				podContainerProbe.UID == fmt.Sprintf("%v", pod.UID) {
				isHit = true
				// diff the current pod container probes vs the npp container probes
				if reflect.DeepEqual(podContainerProbe.Probes, generatePodContainersProbe(pod, containersLivenessProbeConfig)) {
					isChanged = false // no change for probes
					break
				}
				newPodContainerProbes := generatePodContainersProbe(pod, containersLivenessProbeConfig)
				podContainerProbe.Probes = newPodContainerProbes
				break
			}
		}
		if !isHit {
			if len(podNewNppCloneSpec.PodProbes) == 0 {
				podNewNppCloneSpec.PodProbes = []appsv1alpha1.PodProbe{}
			}
			newPodProbe := appsv1alpha1.PodProbe{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				UID:       fmt.Sprintf("%v", pod.UID),
				IP:        pod.Status.PodIP,
				Probes:    generatePodContainersProbe(pod, containersLivenessProbeConfig),
			}
			if len(newPodProbe.Probes) != 0 {
				podNewNppCloneSpec.PodProbes = append(podNewNppCloneSpec.PodProbes, newPodProbe)
			}
		}
		if !isChanged {
			return nil
		}
		nppClone.Spec = *podNewNppCloneSpec
		return r.Client.Update(context.TODO(), nppClone)
	})
	if err != nil {
		klog.Errorf("NodePodProbe update NodePodProbe(%s) failed:%s", pod.Spec.NodeName, err.Error())
		return err
	}
	return nil
}

func (r *ReconcileEnhancedLivenessProbeMapNodeProbe) generateNewNodePodProbe(pod *v1.Pod) (appsv1alpha1.NodePodProbe, error) {
	podLivenessProbeConfig, err := parseEnhancedLivenessProbeConfig(pod)
	if err != nil {
		return appsv1alpha1.NodePodProbe{}, err
	}
	npp_tmp := appsv1alpha1.NodePodProbe{
		ObjectMeta: metav1.ObjectMeta{
			Name: pod.Spec.NodeName,
		},
	}
	npp_tmp.Spec = appsv1alpha1.NodePodProbeSpec{}
	npp_tmp.Spec.PodProbes = []appsv1alpha1.PodProbe{}
	newPodProbe := appsv1alpha1.PodProbe{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		UID:       fmt.Sprintf("%v", pod.UID),
		IP:        pod.Status.PodIP,
		Probes:    generatePodContainersProbe(pod, podLivenessProbeConfig),
	}
	if len(newPodProbe.Probes) != 0 {
		npp_tmp.Spec.PodProbes = append(npp_tmp.Spec.PodProbes, newPodProbe)
	} else {
		return appsv1alpha1.NodePodProbe{}, fmt.Errorf("Failed to generate pod probe object by containers probe config for pod: %s/%s", pod.Namespace, pod.Name)
	}
	return npp_tmp, nil
}

func generatePodContainersProbe(pod *v1.Pod, containersLivenessProbeConfig []livenessprobeUtils.ContainerLivenessProbe) []appsv1alpha1.ContainerProbe {
	newPodContainerProbes := []appsv1alpha1.ContainerProbe{}
	for _, cp := range containersLivenessProbeConfig {
		cProbe := appsv1alpha1.ContainerProbe{}
		cProbe.Name = fmt.Sprintf("%s-%s", pod.Name, cp.Name)
		cProbe.ContainerName = cp.Name
		cProbe.Probe.Probe = cp.LivenessProbe
		newPodContainerProbes = append(newPodContainerProbes, cProbe)
	}
	return newPodContainerProbes
}

func getRawEnhancedLivenessProbeConfig(pod *v1.Pod) string {
	return pod.Annotations[alpha1.AnnotationNativeContainerProbeContext]
}

func parseEnhancedLivenessProbeConfig(pod *v1.Pod) ([]livenessprobeUtils.ContainerLivenessProbe, error) {
	podRawLivenessProbeConfig := getRawEnhancedLivenessProbeConfig(pod)
	if podRawLivenessProbeConfig == "" {
		return []livenessprobeUtils.ContainerLivenessProbe{}, nil
	}
	podLivenessProbeConfig := []livenessprobeUtils.ContainerLivenessProbe{}
	if err := json.Unmarshal([]byte(podRawLivenessProbeConfig), &podLivenessProbeConfig); err != nil {
		klog.Errorf("Failed to json unmarshal pod raw livensss probe configuration, pod: %s/%s, error: %v", pod.Namespace, pod.Name, err)
		return []livenessprobeUtils.ContainerLivenessProbe{}, err
	}
	return podLivenessProbeConfig, nil
}

func usingEnhancedLivenessProbe(pod *v1.Pod) bool {
	if pod.Annotations[alpha1.AnnotationUsingEnhancedLiveness] == "true" {
		return true
	}
	return false
}
