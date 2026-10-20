/*
Copyright 2020 The Kruise Authors.

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

package advancedcronjob

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	clocktesting "k8s.io/utils/clock/testing"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

func init() {
	// Enable klog which is used in dependencies
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", "10")
	fakeClock = clocktesting.NewFakeClock(time.Date(2025, 10, 10, 9, 0, 0, 0, time.UTC))
}

var fakeClock *clocktesting.FakeClock

func TestScheduleWithTimeZone(t *testing.T) {
	cases := []struct {
		schedule     string
		timeZone     *string
		previousTZ   string
		expectedNext string
	}{
		{
			schedule:     "0 10 * * ?",
			timeZone:     nil,
			previousTZ:   "2022-09-05T09:01:00Z",
			expectedNext: "2022-09-05T10:00:00Z",
		},
		{
			schedule:     "0 10 * * ?",
			timeZone:     nil,
			previousTZ:   "2022-09-05T11:01:00Z",
			expectedNext: "2022-09-06T10:00:00Z",
		},
		{
			schedule:     "0 10 * * ?",
			timeZone:     utilpointer.String("Asia/Shanghai"),
			previousTZ:   "2022-09-05T09:01:00Z",
			expectedNext: "2022-09-06T02:00:00Z",
		},
		{
			schedule:     "0 10 * * ?",
			timeZone:     utilpointer.String("Asia/Shanghai"),
			previousTZ:   "2022-09-06T01:01:00Z",
			expectedNext: "2022-09-06T02:00:00Z",
		},
		{
			schedule:     "TZ=Asia/Shanghai 0 10 * * ?",
			timeZone:     nil,
			previousTZ:   "2022-09-06T01:01:00Z",
			expectedNext: "2022-09-06T02:00:00Z",
		},
	}

	for i, tc := range cases {
		acj := &appsv1beta1.AdvancedCronJob{Spec: appsv1beta1.AdvancedCronJobSpec{Schedule: tc.schedule, TimeZone: tc.timeZone}}
		sched, err := cron.ParseStandard(formatSchedule(acj))
		if err != nil {
			t.Fatal(err)
		}

		previousTZ, err := time.Parse(time.RFC3339, tc.previousTZ)
		if err != nil {
			t.Fatal(err)
		}
		gotNext := sched.Next(previousTZ).Format(time.RFC3339)
		if gotNext != tc.expectedNext {
			t.Fatalf("case %d failed, expected next %s, got %s", i, tc.expectedNext, gotNext)
		}
	}
}

// Test scenario:
func TestReconcileAdvancedJobCreateBroadcastJob(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	// A job
	job1 := createJob("job1", broadcastJobTemplate())

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	reconcileJob := createReconcileJobWithBroadcastJobIndex(scheme, job1, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job1",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1beta1.AdvancedCronJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)
	assert.Equal(t, retrievedJob.Status.Type, appsv1beta1.BroadcastJobTemplate)

	brJobList := &appsv1beta1.BroadcastJobList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), brJobList, listOptions)
	assert.NoError(t, err)
}

func TestReconcileAdvancedJobCreateJob(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	// A job
	job1 := createJob("job2", jobTemplate())

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	reconcileJob := createReconcileJobWithBatchJobIndex(scheme, job1, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job2",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1beta1.AdvancedCronJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)
	assert.Equal(t, retrievedJob.Status.Type, appsv1beta1.JobTemplate)

	brJobList := &batchv1.JobList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), brJobList, listOptions)
	assert.NoError(t, err)
}

func createReconcileJobWithBroadcastJobIndex(scheme *runtime.Scheme, initObjs ...client.Object) ReconcileAdvancedCronJob {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(initObjs...).
		WithIndex(&appsv1beta1.BroadcastJob{}, fieldindex.IndexNameForController, func(rawObj client.Object) []string {
			job := rawObj.(*appsv1beta1.BroadcastJob)
			owner := metav1.GetControllerOf(job)
			if owner == nil {
				return nil
			}
			return []string{owner.Name}
		}).WithStatusSubresource(&appsv1beta1.AdvancedCronJob{}).Build()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "advancedcronjob-controller"})
	reconcileJob := ReconcileAdvancedCronJob{
		Client:   fakeClient,
		scheme:   scheme,
		recorder: recorder,
	}
	return reconcileJob
}

func createReconcileJobWithBatchJobIndex(scheme *runtime.Scheme, initObjs ...client.Object) ReconcileAdvancedCronJob {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(initObjs...).
		WithIndex(&batchv1.Job{}, fieldindex.IndexNameForController, func(rawObj client.Object) []string {
			job := rawObj.(*batchv1.Job)
			owner := metav1.GetControllerOf(job)
			if owner == nil {
				return nil
			}
			return []string{owner.Name}
		}).WithStatusSubresource(&appsv1beta1.AdvancedCronJob{}).Build()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "advancedcronjob-controller"})
	reconcileJob := ReconcileAdvancedCronJob{
		Client:   fakeClient,
		scheme:   scheme,
		recorder: recorder,
	}
	return reconcileJob
}

func createNode(nodeName string) *v1.Node {
	node3 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}
	return node3
}

func createJob(jobName string, template appsv1beta1.CronJobTemplate) *appsv1beta1.AdvancedCronJob {
	var successfulHistoryLimit int32 = 2
	var failedHistoryLimit int32 = 1

	paused := false
	job1 := &appsv1beta1.AdvancedCronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              jobName,
			Namespace:         "default",
			UID:               "12345",
			SelfLink:          "/apis/apps.kruise.io/v1beta1/namespaces/default/advancedcronjobs/" + jobName,
			CreationTimestamp: metav1.NewTime(fakeClock.Now()),
		},
		Spec: appsv1beta1.AdvancedCronJobSpec{
			Schedule:                   "*/5 * * * *",
			ConcurrencyPolicy:          "Replace",
			Paused:                     &paused,
			SuccessfulJobsHistoryLimit: &successfulHistoryLimit,
			FailedJobsHistoryLimit:     &failedHistoryLimit,
			Template:                   template,
		},
	}
	return job1
}

func broadcastJobTemplate() appsv1beta1.CronJobTemplate {
	return appsv1beta1.CronJobTemplate{
		BroadcastJobTemplate: &appsv1beta1.BroadcastJobTemplateSpec{
			Spec: appsv1beta1.BroadcastJobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{},
				},
				CompletionPolicy: appsv1beta1.CompletionPolicy{},
				Paused:           false,
				FailurePolicy:    appsv1beta1.FailurePolicy{},
			},
		},
	}
}

func jobTemplate() appsv1beta1.CronJobTemplate {
	return appsv1beta1.CronJobTemplate{
		JobTemplate: &batchv1.JobTemplateSpec{
			Spec: batchv1.JobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{},
				},
			},
		},
	}
}

// Test scenario:
func TestReconcileAdvancedJobCreateImageListPullJob(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	// A job
	job1 := createJob("job1", imageListPullJobTemplate())

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	reconcileJob := createReconcileJobWithImageListPullJobIndex(scheme, job1, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job1",
			Namespace: "default",
		},
	}

	fakeClock.Step(5 * time.Minute)
	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1beta1.AdvancedCronJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)
	assert.Equal(t, appsv1beta1.ImageListPullJobTemplate, retrievedJob.Status.Type)

	jobList := &appsv1beta1.ImageListPullJobList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), jobList, listOptions)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(jobList.Items))
}

// Test scenario:
func TestReconcileAdvancedJobCreateImageListPullJobAgain(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	// A job
	template := imageListPullJobTemplate()
	template.ImageListPullJobTemplate.Annotations = map[string]string{
		"key1": "value1",
	}
	template.ImageListPullJobTemplate.Labels = map[string]string{
		"key1": "value1",
	}
	job1 := createJob("job1", template)
	startingDeadlineSeconds := int64(360)
	job1.Spec.StartingDeadlineSeconds = &startingDeadlineSeconds

	node1 := createNode("node1")
	node2 := createNode("node2")
	node3 := createNode("node3")

	var initObjs []client.Object
	initObjs = append(initObjs, job1, node1, node2, node3)
	initObjs = append(initObjs, createImageListPullJob(-25, 3, 3, 3, job1))
	initObjs = append(initObjs, createImageListPullJob(-20, 2, 1, 2, job1))
	initObjs = append(initObjs, createImageListPullJob(-15, 2, 1, 2, job1))
	initObjs = append(initObjs, createImageListPullJob(-10, 2, 2, 2, job1))
	initObjs = append(initObjs, createImageListPullJob(-5, 2, 2, 2, job1))
	initObjs = append(initObjs, createImageListPullJob(0, 2, 0, 1, job1))
	reconcileJob := createReconcileJobWithImageListPullJobIndex(scheme, initObjs...)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job1",
			Namespace: "default",
		},
	}

	fakeClock.Step(5 * time.Minute)
	_, err := reconcileJob.Reconcile(context.TODO(), request)
	assert.NoError(t, err)
	retrievedJob := &appsv1beta1.AdvancedCronJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)
	assert.Equal(t, appsv1beta1.ImageListPullJobTemplate, retrievedJob.Status.Type)

	jobList := &appsv1beta1.ImageListPullJobList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), jobList, listOptions)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(jobList.Items)) // 1 failed + 2 successful + 1 running
}

func createImageListPullJob(timeDiff int, desired, succeeded, completed int32, parentJob *appsv1beta1.AdvancedCronJob) *appsv1beta1.ImageListPullJob {
	tm := metav1.NewTime(fakeClock.Now().Add(time.Duration(timeDiff) * time.Minute))
	job := &appsv1beta1.ImageListPullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("job1-t%d", timeDiff),
			Namespace: "default",
			UID:       types.UID(fmt.Sprintf("%d", 100+timeDiff)),
			Annotations: map[string]string{
				scheduledTimeAnnotation: tm.Format(time.RFC3339),
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(parentJob, appsv1beta1.SchemeGroupVersion.WithKind("AdvancedCronJob")),
			},
		},
		Spec: appsv1beta1.ImageListPullJobSpec{
			Images: []string{
				"busybox:latest",
				"alpine:latest",
			},
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{},
		},
		Status: appsv1beta1.ImageListPullJobStatus{
			StartTime: &tm,
			Desired:   desired,
			Completed: completed,
			Succeeded: succeeded,
		},
	}
	if desired == completed {
		job.Status.CompletionTime = &tm
	}
	return job
}

func imageListPullJobTemplate() appsv1beta1.CronJobTemplate {
	return appsv1beta1.CronJobTemplate{
		ImageListPullJobTemplate: &appsv1beta1.ImageListPullJobTemplateSpec{
			Spec: appsv1beta1.ImageListPullJobSpec{
				Images: []string{
					"busybox:latest",
					"alpine:latest",
				},
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					Selector: &appsv1beta1.ImagePullJobNodeSelector{
						Names: []string{
							"node2",
							"node3",
						},
					},
				},
			},
		},
	}
}

func createReconcileJobWithImageListPullJobIndex(scheme *runtime.Scheme, initObjs ...client.Object) ReconcileAdvancedCronJob {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(initObjs...).
		WithIndex(&appsv1beta1.ImageListPullJob{}, fieldindex.IndexNameForController, func(rawObj client.Object) []string {
			job := rawObj.(*appsv1beta1.ImageListPullJob)
			owner := metav1.GetControllerOf(job)
			if owner == nil {
				return nil
			}
			return []string{owner.Name}
		}).WithStatusSubresource(&appsv1beta1.AdvancedCronJob{}).Build()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme, v1.EventSource{Component: "advancedcronjob-controller"})
	reconcileJob := ReconcileAdvancedCronJob{
		Client:   fakeClient,
		scheme:   scheme,
		recorder: recorder,
		Clock:    fakeClock,
	}
	return reconcileJob
}

// The recorded lastScheduleTime must survive a reconcile that finds no child job, or only one older
// than it (history limit 0 deletes the newer ones), or every slot since creation counts as missed.
func TestReconcileKeepsLastScheduleTime(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	childMeta := func(owner *appsv1beta1.AdvancedCronJob, at time.Time) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:        "old",
			Namespace:   owner.Namespace,
			Annotations: map[string]string{scheduledTimeAnnotation: at.Format(time.RFC3339)},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, appsv1beta1.SchemeGroupVersion.WithKind("AdvancedCronJob")),
			},
		}
	}
	tests := []struct {
		name          string
		template      appsv1beta1.CronJobTemplate
		now           func() time.Time
		newReconciler func(*runtime.Scheme, ...client.Object) ReconcileAdvancedCronJob
		jobList       client.ObjectList
		failedChild   func(owner *appsv1beta1.AdvancedCronJob, at time.Time) client.Object
	}{
		{"BroadcastJob", broadcastJobTemplate(), time.Now, createReconcileJobWithBroadcastJobIndex, &appsv1beta1.BroadcastJobList{},
			func(owner *appsv1beta1.AdvancedCronJob, at time.Time) client.Object {
				return &appsv1beta1.BroadcastJob{ObjectMeta: childMeta(owner, at), Status: appsv1beta1.BroadcastJobStatus{
					Conditions: []appsv1beta1.JobCondition{{Type: appsv1beta1.JobFailed, Status: v1.ConditionTrue}}}}
			}},
		{"Job", jobTemplate(), time.Now, createReconcileJobWithBatchJobIndex, &batchv1.JobList{},
			func(owner *appsv1beta1.AdvancedCronJob, at time.Time) client.Object {
				return &batchv1.Job{ObjectMeta: childMeta(owner, at), Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: v1.ConditionTrue}}}}
			}},
		{"ImageListPullJob", imageListPullJobTemplate(), fakeClock.Now, createReconcileJobWithImageListPullJobIndex, &appsv1beta1.ImageListPullJobList{},
			func(owner *appsv1beta1.AdvancedCronJob, _ time.Time) client.Object {
				return createImageListPullJob(-150, 2, 1, 2, owner)
			}},
	}
	for _, tc := range tests {
		for _, oldChild := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/oldChild=%v", tc.name, oldChild), func(t *testing.T) {
				now := tc.now()
				lastScheduleTime := metav1.NewTime(now.Add(-2 * time.Minute).Truncate(time.Minute))
				job1 := createJob("job1", tc.template)
				job1.Spec.Schedule = "* * * * *"
				job1.Spec.SuccessfulJobsHistoryLimit = utilpointer.Int32(0)
				job1.Spec.FailedJobsHistoryLimit = utilpointer.Int32(0)
				job1.CreationTimestamp = metav1.NewTime(now.Add(-3 * time.Hour))
				job1.Status.LastScheduleTime = &lastScheduleTime
				initObjs, wantJobs := []client.Object{job1}, 1
				if oldChild {
					job1.Spec.FailedJobsHistoryLimit = utilpointer.Int32(1)
					initObjs, wantJobs = append(initObjs, tc.failedChild(job1, now.Add(-150*time.Minute).Truncate(time.Minute))), 2
				}

				reconcileJob := tc.newReconciler(scheme, initObjs...)
				request := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "job1",
						Namespace: "default",
					},
				}

				_, err := reconcileJob.Reconcile(context.TODO(), request)
				assert.NoError(t, err)
				retrievedJob := &appsv1beta1.AdvancedCronJob{}
				err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
				assert.NoError(t, err)
				if assert.NotNil(t, retrievedJob.Status.LastScheduleTime) {
					assert.Equal(t, lastScheduleTime.Unix(), retrievedJob.Status.LastScheduleTime.Unix())
				}

				err = reconcileJob.List(context.TODO(), tc.jobList, client.InNamespace(request.Namespace))
				assert.NoError(t, err)
				assert.Equal(t, wantJobs, meta.LenList(tc.jobList))
			})
		}
	}
}

func TestWatchJobOwnerEnqueue(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))

	acj := &appsv1beta1.AdvancedCronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-acj",
			Namespace: "default",
			UID:       "test-acj-uid",
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-acj-12345",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(acj, appsv1beta1.SchemeGroupVersion.WithKind("AdvancedCronJob")),
			},
		},
	}

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1beta1.SchemeGroupVersion, batchv1.SchemeGroupVersion})
	mapper.Add(appsv1beta1.SchemeGroupVersion.WithKind("AdvancedCronJob"), meta.RESTScopeNamespace)
	mapper.Add(appsv1beta1.SchemeGroupVersion.WithKind("BroadcastJob"), meta.RESTScopeNamespace)

	// 1. With AdvancedCronJob as ownerType (correct), creating or updating the job enqueues the parent AdvancedCronJob
	h := handler.TypedEnqueueRequestForOwner[*batchv1.Job](scheme, mapper, &appsv1beta1.AdvancedCronJob{}, handler.OnlyControllerOwner())
	q := workqueue.NewTypedRateLimitingQueue[reconcile.Request](workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()

	h.Create(context.TODO(), event.TypedCreateEvent[*batchv1.Job]{Object: job}, q)
	assert.Equal(t, 1, q.Len())
	req, _ := q.Get()
	assert.Equal(t, "test-acj", req.Name)
	assert.Equal(t, "default", req.Namespace)

	// 2. With BroadcastJob as ownerType (the previous bug), nothing is enqueued
	wrongH := handler.TypedEnqueueRequestForOwner[*batchv1.Job](scheme, mapper, &appsv1beta1.BroadcastJob{}, handler.OnlyControllerOwner())
	wrongQ := workqueue.NewTypedRateLimitingQueue[reconcile.Request](workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer wrongQ.ShutDown()

	wrongH.Create(context.TODO(), event.TypedCreateEvent[*batchv1.Job]{Object: job}, wrongQ)
	assert.Equal(t, 0, wrongQ.Len())
}
