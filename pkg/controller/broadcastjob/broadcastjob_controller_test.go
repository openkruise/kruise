/*
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

package broadcastjob

import (
	"flag"
	"fmt"
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func init() {
	// Enable klog which is used in dependencies
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", "10")
}

func TestGetNodeToPodMap(t *testing.T) {
	pods := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p01"},
			Spec:       v1.PodSpec{NodeName: "n01"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p02"},
			Spec: v1.PodSpec{Affinity: &v1.Affinity{NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{{
					MatchFields: []v1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: v1.NodeSelectorOpIn, Values: []string{"n02"}}},
				}}},
			}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p03"},
			Spec:       v1.PodSpec{NodeName: "n03"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p04"},
			Spec: v1.PodSpec{Affinity: &v1.Affinity{NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{{
					MatchFields: []v1.NodeSelectorRequirement{{Key: metav1.ObjectNameField, Operator: v1.NodeSelectorOpIn, Values: []string{"n04"}}},
				}}},
			}}},
		},
	}

	r := &ReconcileBroadcastJob{recorder: record.NewFakeRecorder(10)}
	nodeToPodMap := r.getNodeToPodMap(pods, &appsv1alpha1.BroadcastJob{})
	expectedNodeToPodMap := map[string]*v1.Pod{"n01": pods[0], "n02": pods[1], "n03": pods[2], "n04": pods[3]}
	if !reflect.DeepEqual(nodeToPodMap, expectedNodeToPodMap) {
		t.Fatalf("Unexpected nodeToPodMap: %#v", nodeToPodMap)
	}
}

// Test scenario:
// 1 node with 1 pod running
// 2 nodes without pod running
// parallelism = 2
// 1 new pod created on 1 node
func TestReconcileJobCreatePodAbsolute(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	p := intstr.FromInt(2)
	// A job
	job1 := createJob("job1", p)

	// A POD for job1 running on node1
	job1Pod1onNode1 := createPod(job1, "job1pod1node1", "node1", v1.PodRunning)

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	reconcileJob := createReconcileJob(scheme, job1, job1Pod1onNode1, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job1",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	podList := &v1.PodList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), podList, listOptions)
	assert.NoError(t, err)

	// 2 pods active
	assert.Equal(t, int32(2), retrievedJob.Status.Active)
	// 1 new pod created, because parallelism is 2,
	assert.Equal(t, 2, len(podList.Items))
	// The new pod has the job-name label
	assert.Equal(t, "job1", podList.Items[0].Labels[JobNameLabelKey])
	// 3 desired pods, one for each node
	assert.Equal(t, int32(3), retrievedJob.Status.Desired)
	assert.NotNil(t, retrievedJob.Status.StartTime)
	assert.Equal(t, appsv1alpha1.PhaseRunning, retrievedJob.Status.Phase)
}

// Test scenario:
// 1 node with 1 pod running
// 4 nodes without pod running
// parallelism = 40%
// 1 new pod created on 1 node
func TestReconcileJobCreatePodPercentage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	p := intstr.FromString("40%")
	// A job
	job := createJob("job2", p)

	// A POD for job running on node1
	jobPod1onNode1 := createPod(job, "jobpod1node1", "node1", v1.PodRunning)

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")
	// Node3 does not have pod running
	node4 := createNode("node4")
	// Node3 does not have pod running
	node5 := createNode("node5")

	reconcileJob := createReconcileJob(scheme, job, jobPod1onNode1, node1, node2, node3, node4, node5)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job2",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	podList := &v1.PodList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), podList, listOptions)
	assert.NoError(t, err)

	// 2 pods active
	assert.Equal(t, int32(2), retrievedJob.Status.Active)
	// 1 new pod created, because parallelism is 2,
	assert.Equal(t, 2, len(podList.Items))
	// The new pod has the job-name label
	assert.Equal(t, "job2", podList.Items[0].Labels[JobNameLabelKey])
	// 3 desired pods, one for each node
	assert.Equal(t, int32(5), retrievedJob.Status.Desired)
	assert.NotNil(t, retrievedJob.Status.StartTime)
	assert.Equal(t, appsv1alpha1.PhaseRunning, retrievedJob.Status.Phase)
}

// Test scenario:
// 1 job, 1 normal node, 1 unschedulable node
// Check only 1 pod is created because the other node is unschedulable
func TestPodsOnUnschedulableNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	p := intstr.FromInt(2)
	// A job
	job := createJob("job3", p)

	// Create Node1 with Unschedulable to true
	node1 := createNode("node1")
	node1.Spec.Unschedulable = true

	// Create node2
	node2 := createNode("node2")

	reconcileJob := createReconcileJob(scheme, job, node1, node2)
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job3",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	// assert Job exists
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	podList := &v1.PodList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), podList, listOptions)
	assert.NoError(t, err)

	// 1 pod active on node2,  node1 is unschedulable hence no pod
	assert.Equal(t, int32(1), retrievedJob.Status.Active)
	assert.Equal(t, int32(1), retrievedJob.Status.Desired)
	assert.Equal(t, 1, len(podList.Items))
	assert.Equal(t, appsv1alpha1.PhaseRunning, retrievedJob.Status.Phase)
}

// Test scenario:
// 10 nodes without pods
// 10 pods created with slow start
func TestReconcileJobMultipleBatches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	p := intstr.FromInt(20)
	// A job
	job := createJob("job4", p)

	var objList []client.Object
	objList = append(objList, job)
	for i := 0; i < 10; i++ {
		objList = append(objList, createNode(fmt.Sprintf("node-%d", i)))
	}
	reconcileJob := createReconcileJob(scheme, objList...)
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job4",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	// 10 pods active
	assert.Equal(t, int32(10), retrievedJob.Status.Active)

	podList := &v1.PodList{}
	listOptions := &client.ListOptions{
		Namespace:     request.Namespace,
		LabelSelector: labels.SelectorFromSet(labelsAsMap(job)),
	}
	err = reconcileJob.List(context.TODO(), podList, listOptions)
	assert.NoError(t, err)

	// 10 new pods created
	assert.Equal(t, 10, len(podList.Items))
}

// 3 completed pods, 2 succeeded, 1 failed
// Check job state is failed
func TestJobFailed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	p := intstr.FromInt(10)
	job := createJob("job5", p)
	job.Spec.FailurePolicy.Type = appsv1alpha1.FailurePolicyTypeFailFast

	// Create 3 nodes
	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	// Create 3 pods, 2 succeeded, 1 failed
	pod1onNode1 := createPod(job, "pod1node1", "node1", v1.PodSucceeded)
	pod2onNode2 := createPod(job, "pod2node2", "node2", v1.PodSucceeded)
	pod3onNode3 := createPod(job, "pod3node3", "node3", v1.PodFailed)

	reconcileJob := createReconcileJob(scheme, job, pod1onNode1, pod2onNode2, pod3onNode3, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job5",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	// completionTime is set
	assert.NotNil(t, retrievedJob.Status.CompletionTime)
	// JobComplete condition is set
	assert.Equal(t, appsv1alpha1.JobFailed, retrievedJob.Status.Conditions[len(retrievedJob.Status.Conditions)-1].Type)
	assert.Equal(t, int32(3), retrievedJob.Status.Desired)
	assert.Equal(t, int32(2), retrievedJob.Status.Succeeded)
	assert.Equal(t, int32(1), retrievedJob.Status.Failed)
	assert.Equal(t, int32(0), retrievedJob.Status.Active)
	assert.Equal(t, appsv1alpha1.PhaseFailed, retrievedJob.Status.Phase)
}

// 2 completed pods, 1 succeeded, 1 failed
// FailurePolicy is FailurePolicyTypeContinue
// check job phase is running
func TestJobFailurePolicyTypeContinue(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	p := intstr.FromInt(10)
	job := createJob("job6", p)
	job.Spec.FailurePolicy.Type = appsv1alpha1.FailurePolicyTypeContinue

	// Create 3 nodes
	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	// Create 3 pods, 2 succeeded, 1 failed
	pod1onNode1 := createPod(job, "pod1node1", "node1", v1.PodSucceeded)
	pod3onNode3 := createPod(job, "pod3node3", "node3", v1.PodFailed)

	reconcileJob := createReconcileJob(scheme, job, pod1onNode1, pod3onNode3, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job6",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	assert.Equal(t, int32(3), retrievedJob.Status.Desired)
	assert.Equal(t, int32(1), retrievedJob.Status.Succeeded)
	assert.Equal(t, int32(1), retrievedJob.Status.Failed)
	assert.Equal(t, int32(1), retrievedJob.Status.Active)
	assert.Equal(t, appsv1alpha1.PhaseRunning, retrievedJob.Status.Phase)
}

// 2 completed pods, 1 succeeded, 1 failed
// FailurePolicy is FailurePolicyTypeFailFast
// check job phase is failed
func TestJobFailurePolicyTypeFailFast(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	p := intstr.FromInt(10)
	job := createJob("job7", p)
	job.Spec.FailurePolicy.Type = appsv1alpha1.FailurePolicyTypeFailFast

	// Create 3 nodes
	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	// Create 3 pods, 2 succeeded, 1 failed
	pod1onNode1 := createPod(job, "pod1node1", "node1", v1.PodSucceeded)
	pod3onNode3 := createPod(job, "pod3node3", "node3", v1.PodFailed)

	reconcileJob := createReconcileJob(scheme, job, pod1onNode1, pod3onNode3, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job7",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	assert.Equal(t, int32(3), retrievedJob.Status.Desired)
	assert.Equal(t, int32(1), retrievedJob.Status.Succeeded)
	assert.Equal(t, int32(1), retrievedJob.Status.Failed)
	assert.Equal(t, int32(0), retrievedJob.Status.Active)
	assert.Equal(t, appsv1alpha1.PhaseFailed, retrievedJob.Status.Phase)
}

// 2 completed pods, 1 succeeded, 1 failed
// FailurePolicy is FailurePolicyTypePause
// check job phase is paused
func TestJobFailurePolicyPause(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	p := intstr.FromInt(10)
	job := createJob("job8", p)
	job.Spec.FailurePolicy.Type = appsv1alpha1.FailurePolicyTypePause

	// Create 3 nodes
	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	// Create 2 pods, 1 succeeded, 1 failed
	pod1onNode1 := createPod(job, "pod1node1", "node1", v1.PodSucceeded)
	pod2onNode2 := createPod(job, "pod2node2", "node2", v1.PodFailed)

	reconcileJob := createReconcileJob(scheme, job, pod1onNode1, pod2onNode2, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job8",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	// JobComplete condition is set
	assert.Equal(t, appsv1alpha1.PhasePaused, retrievedJob.Status.Phase)
	assert.Equal(t, int32(3), retrievedJob.Status.Desired)
	assert.Equal(t, int32(1), retrievedJob.Status.Succeeded)
	assert.Equal(t, int32(1), retrievedJob.Status.Failed)
	assert.Equal(t, int32(0), retrievedJob.Status.Active)
}

// 10 nodes, 3 pod, parallelism = 50%
// set paused = true
// check job phase is paused, and no new pod is created.
func TestJobSetPaused(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	p := intstr.FromString("50%")
	// A job
	job := createJob("job9", p)
	job.Spec.Paused = true

	var objList []client.Object
	objList = append(objList, job)
	for i := 0; i < 10; i++ {
		objList = append(objList, createNode(fmt.Sprintf("node-%d", i)))
	}
	// Create 3 succeeded pods
	for i := 0; i < 3; i++ {
		objList = append(objList, createPod(job, fmt.Sprintf("pod%dnode%d", i, i), fmt.Sprintf("node%d", i), v1.PodRunning))
	}

	reconcileJob := createReconcileJob(scheme, objList...)
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job9",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	// 3 pods active
	assert.Equal(t, appsv1alpha1.PhasePaused, retrievedJob.Status.Phase)
	assert.Equal(t, int32(10), retrievedJob.Status.Desired)
	assert.Equal(t, int32(0), retrievedJob.Status.Succeeded)
	assert.Equal(t, int32(0), retrievedJob.Status.Failed)
	assert.Equal(t, int32(3), retrievedJob.Status.Active)
}

// The job should fail after activeDeadline, and active pods will be deleted
func TestJobFailedAfterActiveDeadline(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	p := intstr.FromInt(10)
	// activeDeadline is set 0, to make job fail
	activeDeadline := int64(0)
	now := metav1.Now()
	job := &appsv1alpha1.BroadcastJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job10",
			Namespace: "default",
			UID:       "12345",
			SelfLink:  "/apis/apps.kruise.io/v1alpha1/namespaces/default/broadcastjobs/test",
		},
		Spec: appsv1alpha1.BroadcastJobSpec{
			Parallelism: &p,
			CompletionPolicy: appsv1alpha1.CompletionPolicy{
				ActiveDeadlineSeconds: &activeDeadline,
			},
			FailurePolicy: appsv1alpha1.FailurePolicy{
				Type: appsv1alpha1.FailurePolicyTypeContinue,
			},
		},
		Status: appsv1alpha1.BroadcastJobStatus{
			StartTime: &now,
		},
	}

	node1 := createNode("node1")
	node2 := createNode("node2")

	// two POD for job1 running on node1, node2
	job1Pod1onNode1 := createPod(job, "job1pod1node1", "node1", v1.PodRunning)
	job1Pod2onNode1 := createPod(job, "job1pod2node2", "node2", v1.PodRunning)

	reconcileJob := createReconcileJob(scheme, job, job1Pod1onNode1, job1Pod2onNode1, node1, node2)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job10",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1alpha1.BroadcastJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)

	// The job is failed
	assert.True(t, len(retrievedJob.Status.Conditions) > 0)
	assert.Equal(t, appsv1alpha1.JobFailed, retrievedJob.Status.Conditions[len(retrievedJob.Status.Conditions)-1].Type)
	assert.Equal(t, int32(2), retrievedJob.Status.Failed)
	assert.Equal(t, int32(0), retrievedJob.Status.Active)
	assert.Equal(t, appsv1alpha1.PhaseFailed, retrievedJob.Status.Phase)

	// The active pods are deleted
	podList := &v1.PodList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), podList, listOptions)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(podList.Items))
}

func createReconcileJob(scheme *runtime.Scheme, initObjs ...client.Object) ReconcileBroadcastJob {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "broadcast-controller"})
	reconcileJob := ReconcileBroadcastJob{
		Client:      fakeClient,
		scheme:      scheme,
		recorder:    recorder,
		podModifier: patchPodName,
	}
	return reconcileJob
}

func createNode(nodeName string) *v1.Node {
	node3 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{
				"pods": resource.MustParse("10"),
			},
		},
	}
	return node3
}

func createJob(jobName string, parallelism intstr.IntOrString) *appsv1alpha1.BroadcastJob {
	job1 := &appsv1alpha1.BroadcastJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "default",
			UID:       "12345",
			SelfLink:  "/apis/apps.kruise.io/v1alpha1/namespaces/default/broadcastjobs/" + jobName,
		},
		Spec: appsv1alpha1.BroadcastJobSpec{
			Parallelism:      &parallelism,
			CompletionPolicy: appsv1alpha1.CompletionPolicy{},
		},
	}
	return job1
}

func createPod(job1 *appsv1alpha1.BroadcastJob, podName, nodeName string, phase v1.PodPhase) *v1.Pod {
	job1Pod1onNode1 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Labels:    labelsAsMap(job1),
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job1, appsv1alpha1.SchemeGroupVersion.WithKind("BroadcastJob")),
			},
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
		},
		Status: v1.PodStatus{
			Phase: phase,
		},
	}
	return job1Pod1onNode1
}
func patchPodName(pod *v1.Pod) {
	if pod != nil && pod.Name == "" {
		pod.Name = pod.GenerateName + string(uuid.NewUUID())
	}
}
