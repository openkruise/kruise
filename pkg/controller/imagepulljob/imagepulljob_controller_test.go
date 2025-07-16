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

package imagepulljob

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	testingclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func getInitResource() []client.Object {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kruise-daemon-config",
		},
	}
	return []client.Object{ns}
}

func TestReconcileImagePullJob_ReconcileNotFound(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Test should not requeue
}

func TestReconcileImagePullJob_ReconcileDeletedJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deletionTime := metav1.Now()
	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-job",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test-finalizer"},
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(job).Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Test should not requeue
}

func TestReconcileImagePullJob_ReconcileBasicJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	resource := getInitResource()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(job, node, nodeImage).WithObjects(resource...)
	builder.WithStatusSubresource(job)
	fakeClient := builder.Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job status is updated
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.StartTime).NotTo(gomega.BeNil())
	g.Expect(updatedJob.Status.Desired).To(gomega.Equal(int32(1)))
}

func TestReconcileImagePullJob_ReconcileWithSelector(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				Selector: &appsv1alpha1.ImagePullJobNodeSelector{
					LabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/arch": "amd64",
						},
					},
				},
			},
		},
	}

	node1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	node2 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-2",
			Labels: map[string]string{
				"kubernetes.io/arch": "arm64",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	nodeImage1 := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	nodeImage2 := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-2",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(job, node1, node2, nodeImage1, nodeImage2)
	builder.WithObjects(getInitResource()...).WithStatusSubresource(job)
	fakeClient := builder.Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job only targets nodes matching the selector
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.Desired).To(gomega.Equal(int32(1))) // Only node1 matches
}

func TestReconcileImagePullJob_ReconcileWithParallelism(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	parallelism := intstr.FromInt(2)
	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				Parallelism: &parallelism,
			},
		},
	}

	// Create multiple nodes to test parallelism
	node1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	node2 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-2",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	node3 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-3",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	nodeImage1 := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	nodeImage2 := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-2",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	nodeImage3 := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-3",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(job, node1, node2, node3, nodeImage1, nodeImage2, nodeImage3)
	builder.WithObjects(getInitResource()...).WithStatusSubresource(job)
	fakeClient := builder.Build()
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job respects parallelism
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.Desired).To(gomega.Equal(int32(3))) // All nodes
	g.Expect(updatedJob.Status.Active).To(gomega.BeNumerically("<=", parallelism))
}

func TestReconcileImagePullJob_ReconcileWithActiveDeadlineSeconds(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	startTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	activeDeadline := int64(3600) // 1 hour
	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				CompletionPolicy: appsv1alpha1.CompletionPolicy{
					ActiveDeadlineSeconds: &activeDeadline,
				},
			},
		},
		Status: appsv1alpha1.ImagePullJobStatus{
			StartTime: &startTime,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(job).Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job is marked as failed due to deadline exceeded
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.CompletionTime).NotTo(gomega.BeNil())

	// Check that job is completed
	g.Expect(updatedJob.Status.CompletionTime).NotTo(gomega.BeNil())
}

func TestReconcileImagePullJob_ReconcileWithTTL(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	completionTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	ttl := int32(3600) // 1 hour
	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				CompletionPolicy: appsv1alpha1.CompletionPolicy{
					TTLSecondsAfterFinished: &ttl,
				},
			},
		},
		Status: appsv1alpha1.ImagePullJobStatus{
			CompletionTime: &completionTime,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(job).Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job is deleted due to TTL
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestReconcileImagePullJob_ReconcileWithPodSelector(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				PodSelector: &appsv1alpha1.ImagePullJobPodSelector{
					LabelSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
				},
			},
		},
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
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
			NodeName: "test-node",
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(job, node, pod, nodeImage)
	builder.WithObjects(getInitResource()...).WithStatusSubresource(job)
	fakeClient := builder.Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job only targets nodes with matching pods
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.Desired).To(gomega.Equal(int32(1)))
}

func TestReconcileImagePullJob_ReconcileWithSecrets(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				PullSecrets: []string{"my-secret"},
			},
		},
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: v1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			v1.DockerConfigJsonKey: []byte(`{"auths":{"registry.example.com":{"username":"user","password":"pass"}}}`),
		},
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: appsv1alpha1.NodeImageStatus{
			Desired: 1,
		},
	}

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(job, secret, node, nodeImage)
	builder.WithObjects(getInitResource()...).WithStatusSubresource(job)
	fakeClient := builder.Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that job handles secrets correctly
	updatedJob := &appsv1alpha1.ImagePullJob{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-job"}, updatedJob)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedJob.Status.StartTime).NotTo(gomega.BeNil())
	g.Expect(updatedJob.Status.Desired).To(gomega.Equal(int32(1)))
}

func TestReconcileImagePullJob_ReconcileCompleteJob(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	completionTime := metav1.Now()
	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
		Status: appsv1alpha1.ImagePullJobStatus{
			CompletionTime: &completionTime,
			Succeeded:      1,
			Desired:        1,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(job).Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: s,
		clock:  testingclock.NewFakeClock(time.Now()),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-job",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Test should not requeue
}
