/*
Copyright 2021 The Kruise Authors.

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

package framework

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	imageutils "k8s.io/kubernetes/test/utils/image"
	utilpointer "k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
)

type WorkloadSpreadTester struct {
	C  clientset.Interface
	kc kruiseclientset.Interface
}

func NewWorkloadSpreadTester(c clientset.Interface, kc kruiseclientset.Interface) *WorkloadSpreadTester {
	return &WorkloadSpreadTester{
		C:  c,
		kc: kc,
	}
}

func (t *WorkloadSpreadTester) NewWorkloadSpread(namespace, name string, targetRef *appsv1alpha1.TargetReference, subsets []appsv1alpha1.WorkloadSpreadSubset) *appsv1alpha1.WorkloadSpread {
	return &appsv1alpha1.WorkloadSpread{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: appsv1alpha1.WorkloadSpreadSpec{
			TargetReference: targetRef,
			Subsets:         subsets,
		},
	}
}

func (t *WorkloadSpreadTester) NewBaseCloneSet(namespace string) *appsv1alpha1.CloneSet {
	return &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: appsv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "busybox",
			Namespace: namespace,
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": namespace,
				},
			},
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Type: appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: imageutils.GetE2EImage(imageutils.Httpd),
							//Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
				},
			},
		},
	}
}

func (t *WorkloadSpreadTester) NewBaseHeadlessStatefulSet(namespace string) (*appsv1alpha1.StatefulSet, *corev1.Service) {
	statefulset := &appsv1alpha1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: appsv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "busybox",
			Namespace: namespace,
		},
		Spec: appsv1alpha1.StatefulSetSpec{
			Replicas: utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: imageutils.GetE2EImage(imageutils.Httpd),
							//Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
				},
			},
		},
	}
	headlessSVC := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "busybox",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": namespace,
			},
			ClusterIP: corev1.ClusterIPNone,
			Ports: []corev1.ServicePort{
				{
					Name: "web-port",
					Port: 8080,
				},
			},
		},
	}

	return statefulset, headlessSVC
}

func (t *WorkloadSpreadTester) NewBaseJob(namespace string) *batchv1.Job {
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "busybox",
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Completions: utilpointer.Int32Ptr(10),
			Parallelism: utilpointer.Int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   imageutils.GetE2EImage(imageutils.BusyBox),
							Command: []string{"/bin/sh", "-c", "sleep 5"},
						},
					},
				},
			},
		},
	}
}

func (t *WorkloadSpreadTester) NewBaseDeployment(namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "busybox",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: imageutils.GetE2EImage(imageutils.Httpd),
							//Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
				},
			},
		},
	}
}

func (t *WorkloadSpreadTester) SetNodeLabel(c clientset.Interface, node *corev1.Node, key, value string) {
	labels := node.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[key] = value
	node.SetLabels(labels)
	_, err := c.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *WorkloadSpreadTester) CreateWorkloadSpread(workloadSpread *appsv1alpha1.WorkloadSpread) *appsv1alpha1.WorkloadSpread {
	Logf("create WorkloadSpread (%s/%s)", workloadSpread.Namespace, workloadSpread.Name)
	_, err := t.kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Create(context.TODO(), workloadSpread, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForWorkloadSpreadRunning(workloadSpread)
	Logf("create workloadSpread (%s/%s) success", workloadSpread.Namespace, workloadSpread.Name)
	workloadSpread, _ = t.kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
	return workloadSpread
}

func (t *WorkloadSpreadTester) GetWorkloadSpread(namespace, name string) (*appsv1alpha1.WorkloadSpread, error) {
	Logf("Get WorkloadSpread (%s/%s)", namespace, name)
	return t.kc.AppsV1alpha1().WorkloadSpreads(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *WorkloadSpreadTester) GetCloneSet(namespace, name string) (*appsv1alpha1.CloneSet, error) {
	Logf("Get CloneSet (%s/%s)", namespace, name)
	return t.kc.AppsV1alpha1().CloneSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *WorkloadSpreadTester) WaitForWorkloadSpreadRunning(ws *appsv1alpha1.WorkloadSpread) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.kc.AppsV1alpha1().WorkloadSpreads(ws.Namespace).Get(context.TODO(), ws.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for workloadSpread to enter running: %v", pollErr)
	}
}

func (t *WorkloadSpreadTester) CreateCloneSet(cloneSet *appsv1alpha1.CloneSet) *appsv1alpha1.CloneSet {
	Logf("create CloneSet (%s/%s)", cloneSet.Namespace, cloneSet.Name)
	_, err := t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Create(context.TODO(), cloneSet, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	Logf("create cloneSet (%s/%s) success", cloneSet.Namespace, cloneSet.Name)
	cloneSet, _ = t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Get(context.TODO(), cloneSet.Name, metav1.GetOptions{})
	return cloneSet
}

func (t *WorkloadSpreadTester) CreateStatefulSet(statefulSet *appsv1alpha1.StatefulSet) *appsv1beta1.StatefulSet {
	Logf("create statefulSet (%s/%s)", statefulSet.Namespace, statefulSet.Name)
	_, err := t.kc.AppsV1alpha1().StatefulSets(statefulSet.Namespace).Create(context.TODO(), statefulSet, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	Logf("create statefulSet (%s/%s) success", statefulSet.Namespace, statefulSet.Name)
	asts, _ := t.kc.AppsV1beta1().StatefulSets(statefulSet.Namespace).Get(context.TODO(), statefulSet.Name, metav1.GetOptions{})
	return asts
}

func (t *WorkloadSpreadTester) CreateService(svc *corev1.Service) {
	Logf("create Service (%s/%s)", svc.Namespace, svc.Name)
	_, err := t.C.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *WorkloadSpreadTester) CreateDeployment(deployment *appsv1.Deployment) *appsv1.Deployment {
	Logf("create Deployment (%s/%s)", deployment.Namespace, deployment.Name)
	_, err := t.C.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	Logf("create deployment (%s/%s) success", deployment.Namespace, deployment.Name)
	deployment, _ = t.C.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
	return deployment
}

func (t *WorkloadSpreadTester) CreateJob(job *batchv1.Job) *batchv1.Job {
	Logf("create Deployment (%s/%s)", job.Namespace, job.Name)
	_, err := t.C.BatchV1().Jobs(job.Namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	Logf("create job (%s/%s) success", job.Namespace, job.Name)
	job, _ = t.C.BatchV1().Jobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
	return job
}

func (t *WorkloadSpreadTester) WaitForCloneSetRunning(cloneSet *appsv1alpha1.CloneSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*10,
		func() (bool, error) {
			inner, err := t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Get(context.TODO(), cloneSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration && *inner.Spec.Replicas == inner.Status.ReadyReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneSet to enter running: %v", pollErr)
	}
	Logf("wait cloneSet (%s/%s) running success", cloneSet.Namespace, cloneSet.Name)
}

func (t *WorkloadSpreadTester) WaitForStatefulSetRunning(statefulSet *appsv1beta1.StatefulSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*10,
		func() (bool, error) {
			inner, err := t.kc.AppsV1beta1().StatefulSets(statefulSet.Namespace).Get(context.TODO(), statefulSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration && *inner.Spec.Replicas == inner.Status.ReadyReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for statefulSet to enter running: %v", pollErr)
	}
	Logf("wait statefulSet (%s/%s) running success", statefulSet.Namespace, statefulSet.Name)
}

func (t *WorkloadSpreadTester) WaitForCloneSetRunReplicas(cloneSet *appsv1alpha1.CloneSet, replicas int32) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Get(context.TODO(), cloneSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration &&
				replicas == inner.Status.ReadyReplicas && replicas == inner.Status.UpdatedReadyReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneSet to enter running: %v", pollErr)
	}
}

func (t *WorkloadSpreadTester) WaitForDeploymentRunning(deployment *appsv1.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.C.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration && *inner.Spec.Replicas == inner.Status.ReadyReplicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for deployment to enter running: %v", pollErr)
	}
}

func (t *WorkloadSpreadTester) WaitJobCompleted(job *batchv1.Job) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.C.BatchV1().Jobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Status.Succeeded == *job.Spec.Completions {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for deployment to enter running: %v", pollErr)
	}
}

func (t *WorkloadSpreadTester) GetSelectorPods(namespace string, selector *metav1.LabelSelector) ([]corev1.Pod, error) {
	faster, err := util.ValidatedLabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	podList, err := t.C.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: faster.String()})
	if err != nil {
		return nil, err
	}

	matchedPods := make([]corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		if kubecontroller.IsPodActive(&podList.Items[i]) {
			matchedPods = append(matchedPods, podList.Items[i])
		}
	}
	return matchedPods, nil
}

func (t *WorkloadSpreadTester) UpdateCloneSet(cloneSet *appsv1alpha1.CloneSet) {
	Logf("update cloneSet (%s/%s)", cloneSet.Namespace, cloneSet.Name)
	clone, _ := t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Get(context.TODO(), cloneSet.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone.Spec = cloneSet.Spec
		_, updateErr := t.kc.AppsV1alpha1().CloneSets(clone.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *WorkloadSpreadTester) UpdateDeployment(deployment *appsv1.Deployment) {
	Logf("update deployment (%s/%s)", deployment.Namespace, deployment.Name)
	clone, _ := t.C.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone.Spec = deployment.Spec
		_, updateErr := t.C.AppsV1().Deployments(clone.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		clone, _ = t.C.AppsV1().Deployments(clone.Namespace).Get(context.TODO(), clone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *WorkloadSpreadTester) WaiteCloneSetUpdate(cloneSet *appsv1alpha1.CloneSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Get(context.TODO(), cloneSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			if inner.Generation == inner.Status.ObservedGeneration &&
				*inner.Spec.Replicas == inner.Status.ReadyReplicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.UpdatedReadyReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneSet to enter running: %v", pollErr)
	}
	Logf("wait cloneSet (%s/%s) updated success", cloneSet.Namespace, cloneSet.Name)
}

func (t *WorkloadSpreadTester) WaiteDeploymentUpdate(deployment *appsv1.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.C.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Generation == inner.Status.ObservedGeneration &&
				*inner.Spec.Replicas == inner.Status.ReadyReplicas &&
				*inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneSet to enter running: %v", pollErr)
	}
}

func (t *WorkloadSpreadTester) UpdateWorkloadSpread(workloadSpread *appsv1alpha1.WorkloadSpread) {
	Logf("update workloadSpread (%s/%s)", workloadSpread.Namespace, workloadSpread.Name)
	clone, _ := t.kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone.Spec = workloadSpread.Spec
		_, updateErr := t.kc.AppsV1alpha1().WorkloadSpreads(clone.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		clone, _ = t.kc.AppsV1alpha1().WorkloadSpreads(clone.Namespace).Get(context.TODO(), clone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	pollErr := wait.PollImmediate(time.Second, time.Minute*2, func() (bool, error) {
		clone, err = t.kc.AppsV1alpha1().WorkloadSpreads(clone.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
		if clone.Generation == clone.Status.ObservedGeneration {
			return true, nil
		}
		return false, err
	})
	if pollErr != nil {
		Failf("Failed waiting for workloadSpread to enter ready: %v", pollErr)
	}
}
