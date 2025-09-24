package util

import (
	"context"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

// ObjectManager abstracts the manipulation of Pods and PVCs. The real controller implements this
// with a clientset for writes and listers for reads; for tests we provide stubs.
type ObjectManager interface {
	CreatePod(ctx context.Context, pod *v1.Pod) error
	GetPod(namespace, podName string) (*v1.Pod, error)
	UpdatePod(pod *v1.Pod) error
	DeletePod(pod *v1.Pod) error
	CreateClaim(claim *v1.PersistentVolumeClaim) error
	GetClaim(namespace, claimName string) (*v1.PersistentVolumeClaim, error)
	UpdateClaim(claim *v1.PersistentVolumeClaim) error
	GetStorageClass(scName string) (*storagev1.StorageClass, error)
}

// RealObjectManager uses a clientset.Interface and listers.
type RealObjectManager struct {
	client      clientset.Interface
	podLister   corelisters.PodLister
	claimLister corelisters.PersistentVolumeClaimLister
	scLister    storagelisters.StorageClassLister
}

func NewRealObjectManager(client clientset.Interface, podLister corelisters.PodLister, claimLister corelisters.PersistentVolumeClaimLister, scLister storagelisters.StorageClassLister) *RealObjectManager {
	return &RealObjectManager{
		client:      client,
		podLister:   podLister,
		claimLister: claimLister,
		scLister:    scLister,
	}
}

func (om *RealObjectManager) CreatePod(ctx context.Context, pod *v1.Pod) error {
	_, err := om.client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

func (om *RealObjectManager) GetPod(namespace, podName string) (*v1.Pod, error) {
	return om.podLister.Pods(namespace).Get(podName)
}

func (om *RealObjectManager) UpdatePod(pod *v1.Pod) error {
	_, err := om.client.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
	return err
}

func (om *RealObjectManager) DeletePod(pod *v1.Pod) error {
	return om.client.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
}

func (om *RealObjectManager) CreateClaim(claim *v1.PersistentVolumeClaim) error {
	_, err := om.client.CoreV1().PersistentVolumeClaims(claim.Namespace).Create(context.TODO(), claim, metav1.CreateOptions{})
	return err
}

func (om *RealObjectManager) GetClaim(namespace, claimName string) (*v1.PersistentVolumeClaim, error) {
	return om.claimLister.PersistentVolumeClaims(namespace).Get(claimName)
}

func (om *RealObjectManager) UpdateClaim(claim *v1.PersistentVolumeClaim) error {
	_, err := om.client.CoreV1().PersistentVolumeClaims(claim.Namespace).Update(context.TODO(), claim, metav1.UpdateOptions{})
	return err
}

func (om *RealObjectManager) GetStorageClass(scName string) (*storagev1.StorageClass, error) {
	return om.scLister.Get(scName)
}
