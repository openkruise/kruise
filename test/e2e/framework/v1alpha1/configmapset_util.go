package v1alpha1

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
)

type ConfigMapSetTester struct {
	Kc        kruiseclientset.Interface
	K8sClient clientset.Interface
}

func NewConfigMapSetTester(kc kruiseclientset.Interface, k8sClient clientset.Interface) *ConfigMapSetTester {
	return &ConfigMapSetTester{
		Kc:        kc,
		K8sClient: k8sClient,
	}
}

func (t *ConfigMapSetTester) CreateConfigMapSet(cms *appsv1alpha1.ConfigMapSet) *appsv1alpha1.ConfigMapSet {
	_, err := t.Kc.AppsV1alpha1().ConfigMapSets(cms.Namespace).Create(context.TODO(), cms, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return t.GetConfigMapSet(cms.Namespace, cms.Name)
}

func (t *ConfigMapSetTester) UpdateConfigMapSet(cms *appsv1alpha1.ConfigMapSet) *appsv1alpha1.ConfigMapSet {
	_, err := t.Kc.AppsV1alpha1().ConfigMapSets(cms.Namespace).Update(context.TODO(), cms, metav1.UpdateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return t.GetConfigMapSet(cms.Namespace, cms.Name)
}

func (t *ConfigMapSetTester) GetConfigMapSet(namespace, name string) *appsv1alpha1.ConfigMapSet {
	cms, err := t.Kc.AppsV1alpha1().ConfigMapSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return cms
}

func (t *ConfigMapSetTester) DeleteConfigMapSet(namespace, name string) {
	err := t.Kc.AppsV1alpha1().ConfigMapSets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *ConfigMapSetTester) CreatePod(pod *corev1.Pod) *corev1.Pod {
	_, err := t.K8sClient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return t.GetPod(pod.Namespace, pod.Name)
}

func (t *ConfigMapSetTester) GetPod(namespace, name string) *corev1.Pod {
	pod, err := t.K8sClient.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return pod
}

func (t *ConfigMapSetTester) DeletePod(namespace, name string) {
	err := t.K8sClient.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *ConfigMapSetTester) WaitForConfigMapSetStatus(namespace, name string, condition func(*appsv1alpha1.ConfigMapSet) bool) *appsv1alpha1.ConfigMapSet {
	var finalCMS *appsv1alpha1.ConfigMapSet
	err := wait.PollImmediate(time.Second, time.Minute*3, func() (bool, error) {
		cms := t.GetConfigMapSet(namespace, name)
		if condition(cms) {
			finalCMS = cms
			return true, nil
		}
		return false, nil
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return finalCMS
}

func (t *ConfigMapSetTester) WaitForPodAnnotation(namespace, name string, key string, expectedValue string) *corev1.Pod {
	var finalPod *corev1.Pod
	err := wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
		pod := t.GetPod(namespace, name)
		if pod.Annotations != nil && pod.Annotations[key] == expectedValue {
			finalPod = pod
			return true, nil
		}
		return false, nil
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return finalPod
}
