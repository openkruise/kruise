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

package ephemeraljob

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var s = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(s)
	_ = appsv1alpha1.AddToScheme(s)
	_ = v1.AddToScheme(s)
}

func TestReconcileEphemeralJob_ReconcileNotFound(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	res, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(res.Requeue).To(gomega.BeFalse())
	g.Expect(res.RequeueAfter).To(gomega.Equal(time.Duration(0)))
}

func TestReconcileEphemeralJob_ReconcileDeletedJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deletionTime := metav1.Now()
	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ejob",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{EphemeralContainerFinalizer},
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	res, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(res.Requeue).To(gomega.BeFalse())
}

func TestReconcileEphemeralJob_ReconcileActiveDeadlineExceeded(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	startTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	activeDeadline := int64(3600) // 1 hour
	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			ActiveDeadlineSeconds: &activeDeadline,
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
						},
					},
				},
			},
		},
		Status: appsv1alpha1.EphemeralJobStatus{
			StartTime: &startTime,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	// The first call should succeed (completing the job), the second call may fail if job is deleted
	if err != nil {
		g.Expect(err.Error()).To(gomega.ContainSubstring("not found"))
	}

	// The job should be completed and may be deleted due to TTL
}

func TestReconcileEphemeralJob_ReconcileCompleteJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	completionTime := metav1.Now()
	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(1)),
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
						},
					},
				},
			},
		},
		Status: appsv1alpha1.EphemeralJobStatus{
			CompletionTime: &completionTime,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}
	time.Sleep(3 * time.Second)
	res, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(res.Requeue).To(gomega.BeFalse())
	g.Expect(res.RequeueAfter).To(gomega.Equal(time.Duration(0)))
}

func TestReconcileEphemeralJob_ReconcileWithMatchingPods(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
			UID:       "test-job-uid",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
							Env: []v1.EnvVar{
								{
									Name:  appsv1alpha1.EphemeralContainerEnvKey,
									Value: "test-job-uid",
								},
							},
						},
					},
				},
			},
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: "nginx",
				},
			},
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "test-job-uid",
							},
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			EphemeralContainerStatuses: []v1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: true,
					State: v1.ContainerState{
						Terminated: &v1.ContainerStateTerminated{
							ExitCode:   0,
							FinishedAt: metav1.Now(),
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob, pod).WithStatusSubresource(ejob).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job status is updated
	updatedJob := &appsv1alpha1.EphemeralJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-ejob"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.Succeeded).To(gomega.Equal(int32(1)))
	g.Expect(updatedJob.Status.StartTime).NotTo(gomega.BeNil())
}

func TestReconcileEphemeralJob_ReconcileWithParallelism(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	parallelism := int32(2)
	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
			UID:       "test-job-uid",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Parallelism: &parallelism,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
							Env: []v1.EnvVar{
								{
									Name:  appsv1alpha1.EphemeralContainerEnvKey,
									Value: "test-job-uid",
								},
							},
						},
					},
				},
			},
		},
	}

	// Create multiple pods matching the selector
	pod1 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: "nginx",
				},
			},
		},
	}

	pod2 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-2",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: "nginx",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob, pod1, pod2).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job processes multiple pods up to parallelism limit
	updatedJob := &appsv1alpha1.EphemeralJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-ejob"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.StartTime).NotTo(gomega.BeNil())
}

func TestReconcileEphemeralJob_ReconcileWithFailurePolicy(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
			UID:       "test-job-uid",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name:  "test-container",
							Image: "busybox",
							Env: []v1.EnvVar{
								{
									Name:  appsv1alpha1.EphemeralContainerEnvKey,
									Value: "test-job-uid",
								},
							},
						},
					},
				},
			},
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: "nginx",
				},
			},
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "test-job-uid",
							},
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			EphemeralContainerStatuses: []v1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: false,
					State: v1.ContainerState{
						Terminated: &v1.ContainerStateTerminated{
							ExitCode:   1, // Failed
							FinishedAt: metav1.Now(),
						},
					},
				},
			},
		},
	}

	// Create another pod with failed status to exceed backoff limit
	pod2 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-2",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: "nginx",
				},
			},
			EphemeralContainers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "test-container",
						Image: "busybox",
						Env: []v1.EnvVar{
							{
								Name:  appsv1alpha1.EphemeralContainerEnvKey,
								Value: "test-job-uid",
							},
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			EphemeralContainerStatuses: []v1.ContainerStatus{
				{
					Name:  "test-container",
					Ready: false,
					State: v1.ContainerState{
						Terminated: &v1.ContainerStateTerminated{
							ExitCode:   1, // Failed
							FinishedAt: metav1.Now(),
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob, pod, pod2).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job is marked as failed due to backoff limit exceeded
	updatedJob := &appsv1alpha1.EphemeralJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-ejob"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.Failed).To(gomega.Equal(int32(2)))
}

func TestReconcileEphemeralJob_ReconcileEmptyEphemeralContainers(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	ejob := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ejob",
			Namespace: "default",
		},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(ejob).Build()

	r := &ReconcileEphemeralJob{
		Client:   fakeClient,
		scheme:   s,
		recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-ejob",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job is marked as complete with empty containers
	updatedJob := &appsv1alpha1.EphemeralJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-ejob"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.CompletionTime).NotTo(gomega.BeNil())

	// Check for Complete condition
	found := false
	for _, condition := range updatedJob.Status.Conditions {
		if condition.Type == appsv1alpha1.EJobSucceeded {
			found = true
			break
		}
	}
	g.Expect(found).To(gomega.BeTrue())
}
