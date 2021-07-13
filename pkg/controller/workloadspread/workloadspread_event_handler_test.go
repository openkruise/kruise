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

package workloadspread

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

var (
	deploymentDemo = &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment-test",
			Namespace: "default",
			UID:       types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}

	replicaSetDemo = &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ReplicaSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-test",
			Namespace: "default",
			UID:       types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: pointer.Int32Ptr(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}

	jobDemo = &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-test",
			Namespace: "default",
			UID:       types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
		},
		Spec: batchv1.JobSpec{
			Parallelism: pointer.Int32Ptr(2),
			Completions: pointer.Int32Ptr(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
		},
	}
)

func TestPodEventHandler(t *testing.T) {
	handler := &podEventHandler{}
	injectWorkloadSpread := &wsutil.InjectWorkloadSpread{
		Name:   "test-workloadSpread",
		Subset: "subset-demo",
	}
	by, _ := json.Marshal(injectWorkloadSpread)

	// create
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	createPod := podDemo.DeepCopy()
	createPod.Annotations = map[string]string{
		wsutil.MatchedWorkloadSpreadSubsetAnnotations: string(by),
	}

	createEvt := event.CreateEvent{
		Object: createPod,
	}
	handler.Create(createEvt, createQ)

	if createQ.Len() != 1 {
		t.Errorf("unexpected create event handle queue size, expected 1 actual %d", createQ.Len())
		return
	}

	key, _ := createQ.Get()
	nsn, _ := key.(reconcile.Request)
	if nsn.Namespace != createPod.Namespace && nsn.Name != injectWorkloadSpread.Name {
		t.Errorf("matche WorkloadSpread %s/%s failed", createPod.Namespace, injectWorkloadSpread.Name)
	}

	// update
	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	oldPod := podDemo.DeepCopy()
	oldPod.Annotations = map[string]string{
		wsutil.MatchedWorkloadSpreadSubsetAnnotations: string(by),
	}
	newPod := podDemo.DeepCopy()
	newPod.Annotations = map[string]string{
		wsutil.MatchedWorkloadSpreadSubsetAnnotations: string(by),
	}
	newPod.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	updateEvt := event.UpdateEvent{
		ObjectOld: oldPod,
		ObjectNew: newPod,
	}
	handler.Update(updateEvt, updateQ)

	if updateQ.Len() != 1 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", createQ.Len())
		return
	}

	key, _ = updateQ.Get()
	nsn, _ = key.(reconcile.Request)
	if nsn.Namespace != newPod.Namespace && nsn.Name != injectWorkloadSpread.Name {
		t.Errorf("matche WorkloadSpread %s/%s failed", newPod.Namespace, injectWorkloadSpread.Name)
	}

	// delete
	deleteQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	deletePod := podDemo.DeepCopy()
	deletePod.Annotations = map[string]string{
		wsutil.MatchedWorkloadSpreadSubsetAnnotations: string(by),
	}

	deleteEvt := event.DeleteEvent{
		Object: deletePod,
	}
	handler.Delete(deleteEvt, deleteQ)

	if deleteQ.Len() != 1 {
		t.Errorf("unexpected delete event handle queue size, expected 1 actual %d", deleteQ.Len())
		return
	}

	key, _ = deleteQ.Get()
	nsn, _ = key.(reconcile.Request)
	if nsn.Namespace != deletePod.Namespace && nsn.Name != injectWorkloadSpread.Name {
		t.Errorf("matche WorkloadSpread %s/%s failed", deletePod.Namespace, injectWorkloadSpread.Name)
	}
}

func TestGetWorkloadSpreadForCloneSet(t *testing.T) {
	cases := []struct {
		name                 string
		getCloneSet          func() *appsalphav1.CloneSet
		getWorkloadSpreads   func() []*appsalphav1.WorkloadSpread
		expectWorkloadSpread func() *appsalphav1.WorkloadSpread
	}{
		{
			name: "no matched WorkloadSpread",
			getCloneSet: func() *appsalphav1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.Spec.TargetReference = nil

				workloadSpread2 := workloadSpreadDemo.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference.APIVersion = "apps/v1"

				workloadSpread3 := workloadSpreadDemo.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference.Kind = "Deployment"

				workloadSpread4 := workloadSpreadDemo.DeepCopy()
				workloadSpread4.Name = "ws-4"
				workloadSpread4.Spec.TargetReference.Name = "test"

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3, workloadSpread4}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "deletionTimestamp is not nil, no matched WorkloadSpread",
			getCloneSet: func() *appsalphav1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.DeletionTimestamp = &metav1.Time{Time: time.Now()}

				workloadSpread2 := workloadSpreadDemo.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := workloadSpreadDemo.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "matched WorkloadSpread ws-1",
			getCloneSet: func() *appsalphav1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"

				workloadSpread2 := workloadSpreadDemo.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := workloadSpreadDemo.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"
				return workloadSpread1
			},
		},
		{
			name: "different version, matched WorkloadSpread ws-1",
			getCloneSet: func() *appsalphav1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.Spec.TargetReference.APIVersion = "apps.kruise.io/v1"

				workloadSpread2 := workloadSpreadDemo.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := workloadSpreadDemo.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"
				return workloadSpread1
			},
		},
		{
			name: "matched WorkloadSpread ws-3",
			getCloneSet: func() *appsalphav1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.Spec.TargetReference = nil

				workloadSpread2 := workloadSpreadDemo.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := workloadSpreadDemo.DeepCopy()
				workloadSpread3.Name = "ws-3"

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				workloadSpread1 := workloadSpreadDemo.DeepCopy()
				workloadSpread1.Name = "ws-3"
				return workloadSpread1
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme)
			for _, ws := range cs.getWorkloadSpreads() {
				newWorkloadSpread := ws.DeepCopy()
				err := fakeClient.Create(context.TODO(), newWorkloadSpread)
				if err != nil {
					t.Fatalf("create WorkloadSpread failed: %s", err.Error())
				}
			}

			nsn := types.NamespacedName{
				Namespace: cs.getCloneSet().Namespace,
				Name:      cs.getCloneSet().Name,
			}
			handler := workloadEventHandler{Reader: fakeClient}
			workloadSpread, _ := handler.getWorkloadSpreadForWorkload(nsn, controllerKruiseKindCS)
			expectTopology := cs.expectWorkloadSpread()

			if expectTopology == nil {
				if workloadSpread != nil {
					t.Fatalf("get WorkloadSpread for CloneSet failed")
				}
			} else {
				if workloadSpread == nil || workloadSpread.Name != expectTopology.Name {
					t.Fatalf("get WorkloadSpread for CloneSet failed")
				}
			}
		})
	}
}

func TestGetWorkloadSpreadForDeployment(t *testing.T) {
	targetRef := appsalphav1.TargetReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "deployment-test",
	}
	ws := workloadSpreadDemo.DeepCopy()
	ws.Spec.TargetReference = &targetRef

	cases := []struct {
		name                 string
		getDeployment        func() *appsv1.Deployment
		getWorkloadSpreads   func() []*appsalphav1.WorkloadSpread
		expectWorkloadSpread func() *appsalphav1.WorkloadSpread
	}{
		{
			name: "no matched WorkloadSpread",
			getDeployment: func() *appsv1.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.Spec.TargetReference = nil

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference.APIVersion = "apps.kruise.io/v1"

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference.Kind = "CloneSet"

				workloadSpread4 := ws.DeepCopy()
				workloadSpread4.Name = "ws-4"
				workloadSpread4.Spec.TargetReference.Name = "test"

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3, workloadSpread4}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "deletionTimestamp is not nil, no matched WorkloadSpread",
			getDeployment: func() *appsv1.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.DeletionTimestamp = &metav1.Time{Time: time.Now()}

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "matched WorkloadSpread ws-1",
			getDeployment: func() *appsv1.Deployment {
				return deploymentDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				workloadSpread := ws.DeepCopy()
				workloadSpread.Name = "ws-1"
				return workloadSpread
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme)
			for _, ws := range cs.getWorkloadSpreads() {
				newWorkloadSpread := ws.DeepCopy()
				err := fakeClient.Create(context.TODO(), newWorkloadSpread)
				if err != nil {
					t.Fatalf("create WorkloadSpread failed: %s", err.Error())
				}
			}

			nsn := types.NamespacedName{
				Namespace: cs.getDeployment().Namespace,
				Name:      cs.getDeployment().Name,
			}
			handler := workloadEventHandler{Reader: fakeClient}
			workloadSpread, _ := handler.getWorkloadSpreadForWorkload(nsn, controllerKindDep)
			expectTopology := cs.expectWorkloadSpread()

			if expectTopology == nil {
				if workloadSpread != nil {
					t.Fatalf("get WorkloadSpread for Deployment failed")
				}
			} else {
				if workloadSpread == nil || workloadSpread.Name != expectTopology.Name {
					t.Fatalf("get WorkloadSpread for Deployment failed")
				}
			}
		})
	}
}

func TestGetWorkloadSpreadForJob(t *testing.T) {
	targetRef := appsalphav1.TargetReference{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       "job-test",
	}
	ws := workloadSpreadDemo.DeepCopy()
	ws.Spec.TargetReference = &targetRef

	cases := []struct {
		name                 string
		getJob               func() *batchv1.Job
		getWorkloadSpreads   func() []*appsalphav1.WorkloadSpread
		expectWorkloadSpread func() *appsalphav1.WorkloadSpread
	}{
		{
			name: "no matched WorkloadSpread",
			getJob: func() *batchv1.Job {
				return jobDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.Spec.TargetReference = nil

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference.APIVersion = "apps.kruise.io/v1"

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference.Kind = "CloneSet"

				workloadSpread4 := ws.DeepCopy()
				workloadSpread4.Name = "ws-4"
				workloadSpread4.Spec.TargetReference.Name = "test"

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3, workloadSpread4}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "matched WorkloadSpread ws-1",
			getJob: func() *batchv1.Job {
				return jobDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				workloadSpread := ws.DeepCopy()
				workloadSpread.Name = "ws-1"
				return workloadSpread
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme)
			for _, ws := range cs.getWorkloadSpreads() {
				newWorkloadSpread := ws.DeepCopy()
				err := fakeClient.Create(context.TODO(), newWorkloadSpread)
				if err != nil {
					t.Fatalf("create WorkloadSpread failed: %s", err.Error())
				}
			}

			nsn := types.NamespacedName{
				Namespace: cs.getJob().Namespace,
				Name:      cs.getJob().Name,
			}
			handler := workloadEventHandler{Reader: fakeClient}
			workloadSpread, _ := handler.getWorkloadSpreadForWorkload(nsn, controllerKindJob)
			expectTopology := cs.expectWorkloadSpread()

			if expectTopology == nil {
				if workloadSpread != nil {
					t.Fatalf("get WorkloadSpread for Job failed")
				}
			} else {
				if workloadSpread == nil || workloadSpread.Name != expectTopology.Name {
					t.Fatalf("get WorkloadSpread for Job failed")
				}
			}
		})
	}
}

func TestGetWorkloadSpreadForReplicaSet(t *testing.T) {
	targetRef := appsalphav1.TargetReference{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "rs-test",
	}
	ws := workloadSpreadDemo.DeepCopy()
	ws.Spec.TargetReference = &targetRef

	cases := []struct {
		name                 string
		getReplicaset        func() *appsv1.ReplicaSet
		getWorkloadSpreads   func() []*appsalphav1.WorkloadSpread
		expectWorkloadSpread func() *appsalphav1.WorkloadSpread
	}{
		{
			name: "no matched WorkloadSpread",
			getReplicaset: func() *appsv1.ReplicaSet {
				return replicaSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.Spec.TargetReference = nil

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference.APIVersion = "apps.kruise.io/v1"

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference.Kind = "CloneSet"

				workloadSpread4 := ws.DeepCopy()
				workloadSpread4.Name = "ws-4"
				workloadSpread4.Spec.TargetReference.Name = "test"

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3, workloadSpread4}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "deletionTimestamp is not nil, no matched WorkloadSpread",
			getReplicaset: func() *appsv1.ReplicaSet {
				return replicaSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"
				workloadSpread1.DeletionTimestamp = &metav1.Time{Time: time.Now()}

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				return nil
			},
		},
		{
			name: "matched WorkloadSpread ws-1",
			getReplicaset: func() *appsv1.ReplicaSet {
				return replicaSetDemo.DeepCopy()
			},
			getWorkloadSpreads: func() []*appsalphav1.WorkloadSpread {
				workloadSpread1 := ws.DeepCopy()
				workloadSpread1.Name = "ws-1"

				workloadSpread2 := ws.DeepCopy()
				workloadSpread2.Name = "ws-2"
				workloadSpread2.Spec.TargetReference = nil

				workloadSpread3 := ws.DeepCopy()
				workloadSpread3.Name = "ws-3"
				workloadSpread3.Spec.TargetReference = nil

				return []*appsalphav1.WorkloadSpread{workloadSpread1, workloadSpread2, workloadSpread3}
			},
			expectWorkloadSpread: func() *appsalphav1.WorkloadSpread {
				workloadSpread := ws.DeepCopy()
				workloadSpread.Name = "ws-1"
				return workloadSpread
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme)
			for _, ws := range cs.getWorkloadSpreads() {
				newWorkloadSpread := ws.DeepCopy()
				err := fakeClient.Create(context.TODO(), newWorkloadSpread)
				if err != nil {
					t.Fatalf("create WorkloadSpread failed: %s", err.Error())
				}
			}

			nsn := types.NamespacedName{
				Namespace: cs.getReplicaset().Namespace,
				Name:      cs.getReplicaset().Name,
			}
			handler := workloadEventHandler{Reader: fakeClient}
			workloadSpread, _ := handler.getWorkloadSpreadForWorkload(nsn, controllerKindRS)
			expectTopology := cs.expectWorkloadSpread()

			if expectTopology == nil {
				if workloadSpread != nil {
					t.Fatalf("get WorkloadSpread for ReplicaSet failed")
				}
			} else {
				if workloadSpread == nil || workloadSpread.Name != expectTopology.Name {
					t.Fatalf("get WorkloadSpread for ReplicaSet failed")
				}
			}
		})
	}
}

func TestWorkloadEventHandlerForCreate(t *testing.T) {
	fakeClient := fake.NewFakeClientWithScheme(scheme, workloadSpreadDemo.DeepCopy())
	handler := &workloadEventHandler{Reader: fakeClient}

	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createCloneSet := cloneSetDemo.DeepCopy()
	createEvt := event.CreateEvent{
		Object: createCloneSet,
		Meta:   &createCloneSet.ObjectMeta,
	}
	handler.Create(createEvt, createQ)
	if createQ.Len() != 1 {
		t.Errorf("unexpected create event handle queue size, expected 1 actual %d", createQ.Len())
		return
	}
	key, _ := createQ.Get()
	nsn, _ := key.(reconcile.Request)
	if nsn.Namespace != workloadSpreadDemo.Namespace && nsn.Name != workloadSpreadDemo.Name {
		t.Errorf("match WorkloadSpread %s/%s failed", workloadSpreadDemo.Namespace, workloadSpreadDemo.Name)
	}
}

func TestWorkloadEventHandlerForDelete(t *testing.T) {
	targetRef := appsalphav1.TargetReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "deployment-test",
	}
	ws := workloadSpreadDemo.DeepCopy()
	ws.Spec.TargetReference = &targetRef

	fakeClient := fake.NewFakeClientWithScheme(scheme, ws.DeepCopy())
	handler := &workloadEventHandler{Reader: fakeClient}

	deleteQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	deleteDeployment := deploymentDemo.DeepCopy()
	deleteEvt := event.DeleteEvent{
		Object: deleteDeployment,
		Meta:   &deleteDeployment.ObjectMeta,
	}
	handler.Delete(deleteEvt, deleteQ)
	if deleteQ.Len() != 1 {
		t.Errorf("unexpected delete event handle queue size, expected 1 actual %d", deleteQ.Len())
		return
	}
	key, _ := deleteQ.Get()
	nsn, _ := key.(reconcile.Request)
	if nsn.Namespace != ws.Namespace && nsn.Name != ws.Name {
		t.Errorf("match WorkloadSpread %s/%s failed", ws.Namespace, ws.Name)
	}
}

func TestWorkloadEventHandlerForUpdate(t *testing.T) {
	targetRef := appsalphav1.TargetReference{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "rs-test",
	}
	ws := workloadSpreadDemo.DeepCopy()
	ws.Spec.TargetReference = &targetRef

	fakeClient := fake.NewFakeClientWithScheme(scheme, ws.DeepCopy())
	handler := &workloadEventHandler{Reader: fakeClient}

	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	oldReplicaSet := replicaSetDemo.DeepCopy()
	newReplicaSet := replicaSetDemo.DeepCopy()
	newReplicaSet.Spec.Replicas = pointer.Int32Ptr(*(oldReplicaSet.Spec.Replicas) + 1)

	updateEvt := event.UpdateEvent{
		ObjectOld: oldReplicaSet,
		ObjectNew: newReplicaSet,
		MetaOld:   &oldReplicaSet.ObjectMeta,
		MetaNew:   &newReplicaSet.ObjectMeta,
	}
	handler.Update(updateEvt, updateQ)
	if updateQ.Len() != 1 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
		return
	}
	key, _ := updateQ.Get()
	nsn, _ := key.(reconcile.Request)
	if nsn.Namespace != ws.Namespace && nsn.Name != ws.Name {
		t.Errorf("match WorkloadSpread %s/%s failed", ws.Namespace, ws.Name)
	}
}
