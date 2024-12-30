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
	"testing"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

func init() {
	// Enable klog which is used in dependencies
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", "10")
}

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
		acj := &appsv1alpha1.AdvancedCronJob{Spec: appsv1alpha1.AdvancedCronJobSpec{Schedule: tc.schedule, TimeZone: tc.timeZone}}
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
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
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
	retrievedJob := &appsv1alpha1.AdvancedCronJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)
	assert.Equal(t, retrievedJob.Status.Type, appsv1alpha1.BroadcastJobTemplate)

	brJobList := &appsv1alpha1.BroadcastJobList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), brJobList, listOptions)
	assert.NoError(t, err)
}

func TestReconcileAdvancedJobCreateJob(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
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
	retrievedJob := &appsv1alpha1.AdvancedCronJob{}
	err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
	assert.NoError(t, err)
	assert.Equal(t, retrievedJob.Status.Type, appsv1alpha1.JobTemplate)

	brJobList := &batchv1.JobList{}
	listOptions := client.InNamespace(request.Namespace)
	err = reconcileJob.List(context.TODO(), brJobList, listOptions)
	assert.NoError(t, err)
}

func createReconcileJobWithBroadcastJobIndex(scheme *runtime.Scheme, initObjs ...client.Object) ReconcileAdvancedCronJob {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(initObjs...).
		WithIndex(&appsv1alpha1.BroadcastJob{}, fieldindex.IndexNameForController, func(rawObj client.Object) []string {
			job := rawObj.(*appsv1alpha1.BroadcastJob)
			owner := metav1.GetControllerOf(job)
			if owner == nil {
				return nil
			}
			return []string{owner.Name}
		}).WithStatusSubresource(&appsv1alpha1.AdvancedCronJob{}).Build()
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
		}).WithStatusSubresource(&appsv1alpha1.AdvancedCronJob{}).Build()
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

func createJob(jobName string, template appsv1alpha1.CronJobTemplate) *appsv1alpha1.AdvancedCronJob {
	var historyLimit int32 = 3

	paused := false
	job1 := &appsv1alpha1.AdvancedCronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "default",
			UID:       "12345",
			SelfLink:  "/apis/apps.kruise.io/v1alpha1/namespaces/default/advancedcronjobs/" + jobName,
		},
		Spec: appsv1alpha1.AdvancedCronJobSpec{
			Schedule:                   "* * * * *",
			ConcurrencyPolicy:          "Replace",
			Paused:                     &paused,
			SuccessfulJobsHistoryLimit: &historyLimit,
			FailedJobsHistoryLimit:     &historyLimit,
			Template:                   template,
		},
	}
	return job1
}

func broadcastJobTemplate() appsv1alpha1.CronJobTemplate {
	return appsv1alpha1.CronJobTemplate{
		BroadcastJobTemplate: &appsv1alpha1.BroadcastJobTemplateSpec{
			Spec: appsv1alpha1.BroadcastJobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{},
				},
				CompletionPolicy: appsv1alpha1.CompletionPolicy{},
				Paused:           false,
				FailurePolicy:    appsv1alpha1.FailurePolicy{},
			},
		},
	}
}

func jobTemplate() appsv1alpha1.CronJobTemplate {
	return appsv1alpha1.CronJobTemplate{
		JobTemplate: &batchv1.JobTemplateSpec{
			Spec: batchv1.JobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{},
				},
			},
		},
	}
}
