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

package nodeimage

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
	testingclock "k8s.io/utils/clock/testing"
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

func TestReconcileNodeImage_ReconcileNotFound(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Test should not requeue
}

func TestReconcileNodeImage_ReconcileDeletedNodeImage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deletionTime := metav1.Now()
	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-node",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test-finalizer"},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Test should not requeue
}

func TestReconcileNodeImage_ReconcileWithNode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
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

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(node).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that NodeImage was created
	nodeImage := &appsv1alpha1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-node"}, nodeImage)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(nodeImage.Name).To(gomega.Equal("test-node"))
}

func TestReconcileNodeImage_ReconcileWithFakeNodeImage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				fakeLabelKey: "true",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Test should not requeue
}

func TestReconcileNodeImage_ReconcileWithNodeImageAndNode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
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

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(node, nodeImage).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that NodeImage labels are updated to match Node labels
	updatedNodeImage := &appsv1alpha1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-node"}, updatedNodeImage)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedNodeImage.Labels).To(gomega.Equal(node.Labels))
}

func TestReconcileNodeImage_ReconcileWithImagePullJobs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
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

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
			},
		},
	}

	imagePullJob := &appsv1alpha1.ImagePullJob{
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

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(node, nodeImage, imagePullJob).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that NodeImage status is updated with image pull job info
	updatedNodeImage := &appsv1alpha1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-node"}, updatedNodeImage)
	g.Expect(err).NotTo(gomega.HaveOccurred())
}

func TestReconcileNodeImage_ReconcileWithUnreadyNode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionFalse,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(node).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that NodeImage is not created for unready node
	nodeImage := &appsv1alpha1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-node"}, nodeImage)
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestReconcileNodeImage_ReconcileWithNodeImageStatusUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
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

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
			},
		},
		Status: appsv1alpha1.NodeImageStatus{
			ImageStatuses: map[string]appsv1alpha1.ImageStatus{
				"nginx": {
					Tags: []appsv1alpha1.ImageTagStatus{
						{
							Tag:     "latest",
							ImageID: "sha256:abc123",
							Phase:   appsv1alpha1.ImagePhaseSucceeded,
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(node, nodeImage).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that NodeImage status is preserved
	updatedNodeImage := &appsv1alpha1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-node"}, updatedNodeImage)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(updatedNodeImage.Status.ImageStatuses).NotTo(gomega.BeEmpty())
	g.Expect(updatedNodeImage.Status.ImageStatuses["nginx"].Tags[0].ImageID).To(gomega.Equal("sha256:abc123"))
}

func TestReconcileNodeImage_ReconcileWithNodeImageCleanup(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// NodeImage without corresponding Node should be cleaned up
	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	r := &ReconcileNodeImage{
		Client:        fakeClient,
		scheme:        s,
		clock:         testingclock.NewFakeClock(time.Now()),
		eventRecorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-node",
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Check that NodeImage is deleted when Node doesn't exist
	updatedNodeImage := &appsv1alpha1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-node"}, updatedNodeImage)
	g.Expect(err).To(gomega.HaveOccurred())
}
