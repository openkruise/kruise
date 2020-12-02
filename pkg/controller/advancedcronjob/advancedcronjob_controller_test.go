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
	"flag"
	"testing"

	batchv1 "k8s.io/api/batch/v1"

	batchv1beta1 "k8s.io/api/batch/v1beta1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
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

// Test scenario:
func TestReconcileAdvancedJobCreateBroadcastJob(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	job1 := createJob("job1", broadcastJobTemplate())

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	reconcileJob := createReconcileJob(scheme, job1, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job1",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(request)
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
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = batchv1beta1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	// A job
	job1 := createJob("job2", jobTemplate())

	// Node1 has 1 pod running
	node1 := createNode("node1")
	// Node2 does not have pod running
	node2 := createNode("node2")
	// Node3 does not have pod running
	node3 := createNode("node3")

	reconcileJob := createReconcileJob(scheme, job1, node1, node2, node3)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "job2",
			Namespace: "default",
		},
	}

	_, err := reconcileJob.Reconcile(request)
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

func createReconcileJob(scheme *runtime.Scheme, initObjs ...runtime.Object) ReconcileAdvancedCronJob {
	fakeClient := fake.NewFakeClientWithScheme(scheme, initObjs...)
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
	var historyLimit int32
	historyLimit = 3

	var paused bool
	paused = false
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
		JobTemplate: &batchv1beta1.JobTemplateSpec{
			Spec: batchv1.JobSpec{
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{},
				},
			},
		},
	}
}
