/*
Copyright 2023 The Kruise Authors.

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

package imagelistpulljob

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

var testscheme *k8sruntime.Scheme

var (
	images = []string{"nginx:1.9.1", "busybox:1.35", "httpd:2.4.38"}
	jobUID = "123"
)

func init() {
	testscheme = k8sruntime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(testscheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(testscheme))
}

func TestReconcile(t *testing.T) {
	now := metav1.Now()
	instance := &appsv1alpha1.ImageListPullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", UID: types.UID(jobUID)},
		Spec: appsv1alpha1.ImageListPullJobSpec{
			Images: images,
		},
	}
	cases := []struct {
		name               string
		expectImagePullJob []*appsv1alpha1.ImagePullJob
	}{
		{
			name: "test-imagelistpulljob-controller",
			expectImagePullJob: []*appsv1alpha1.ImagePullJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j01",
						Namespace: instance.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: instance.Name},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: images[0],
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					},
					Status: appsv1alpha1.ImagePullJobStatus{
						Active:         0,
						StartTime:      &now,
						CompletionTime: &now,
						Succeeded:      1,
						Failed:         0,
						Desired:        1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j02",
						Namespace: instance.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: instance.Name},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: images[1],
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					},
					Status: appsv1alpha1.ImagePullJobStatus{
						Desired:   1,
						Active:    1,
						StartTime: &now,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j03",
						Namespace: instance.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: instance.Name},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: images[2],
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					},
					Status: appsv1alpha1.ImagePullJobStatus{
						Desired:        1,
						Active:         0,
						StartTime:      &now,
						CompletionTime: &now,
						Failed:         1,
						Succeeded:      0,
						FailedNodes:    []string{"1.1.1.1"},
					},
				},
			},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			reconcileJob := createReconcileJob(testscheme, instance, cs.expectImagePullJob[0], cs.expectImagePullJob[1], cs.expectImagePullJob[2])

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			}

			_, err := reconcileJob.Reconcile(context.TODO(), request)
			assert.NoError(t, err)
			retrievedJob := &appsv1alpha1.ImageListPullJob{}
			err = reconcileJob.Get(context.TODO(), request.NamespacedName, retrievedJob)
			assert.NoError(t, err)

			imagePullJobList := &appsv1alpha1.ImagePullJobList{}
			listOptions := client.InNamespace(request.Namespace)
			err = reconcileJob.List(context.TODO(), imagePullJobList, listOptions)
			assert.NoError(t, err)
			assert.Equal(t, int32(len(images)), retrievedJob.Status.Desired)
			assert.Equal(t, int32(1), retrievedJob.Status.Active)
			assert.Equal(t, int32(1), retrievedJob.Status.Succeeded)
			assert.Equal(t, int32(2), retrievedJob.Status.Completed)
			assert.Equal(t, 1, len(retrievedJob.Status.FailedImageStatuses))
			for _, job := range imagePullJobList.Items {
				assert.Contains(t, instance.Spec.Images, job.Spec.Image)
			}
		})
	}
}

func TestComputeImagePullJobActions(t *testing.T) {
	baseImageListPullJob := &appsv1alpha1.ImageListPullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", UID: types.UID(jobUID)},
	}
	cases := []struct {
		name                      string
		ImageListPullJob          *appsv1alpha1.ImageListPullJob
		ImagePullJobs             map[string]*appsv1alpha1.ImagePullJob
		needToCreateImagePullJobs []*appsv1alpha1.ImagePullJob
		needToDeleteImagePullJobs []*appsv1alpha1.ImagePullJob
	}{
		{
			name: "test_add_image",
			ImageListPullJob: &appsv1alpha1.ImageListPullJob{
				ObjectMeta: baseImageListPullJob.ObjectMeta,
				Spec: appsv1alpha1.ImageListPullJobSpec{
					Images: []string{"nginx:1.9.1", "busybox:1.35", "httpd:2.4.38"},
					ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
						CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
					},
				},
			},
			ImagePullJobs: map[string]*appsv1alpha1.ImagePullJob{
				"nginx:1.9.1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j01",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "nginx:1.9.1",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
				"busybox:1.35": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j02",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "busybox:1.35",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
			},
			needToCreateImagePullJobs: []*appsv1alpha1.ImagePullJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    "default",
						GenerateName: "foo-",
						Labels:       make(map[string]string),
						Annotations:  make(map[string]string),
						OwnerReferences: []metav1.OwnerReference{
							*metav1.NewControllerRef(baseImageListPullJob, controllerKind),
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "httpd:2.4.38",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					},
				},
			},
			needToDeleteImagePullJobs: nil,
		},
		{
			name: "test_delete_image",
			ImageListPullJob: &appsv1alpha1.ImageListPullJob{
				ObjectMeta: baseImageListPullJob.ObjectMeta,
				Spec: appsv1alpha1.ImageListPullJobSpec{
					Images: []string{"nginx:1.9.1"},
					ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
						CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
					},
				},
			},
			ImagePullJobs: map[string]*appsv1alpha1.ImagePullJob{
				"nginx:1.9.1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j01",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "nginx:1.9.1",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
				"busybox:1.35": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j02",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "busybox:1.35",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
			},
			needToCreateImagePullJobs: nil,
			needToDeleteImagePullJobs: []*appsv1alpha1.ImagePullJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j02",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "busybox:1.35",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
			},
		},
		{
			name: "test_change_imagepulljobtemplate",
			ImageListPullJob: &appsv1alpha1.ImageListPullJob{
				ObjectMeta: baseImageListPullJob.ObjectMeta,
				Spec: appsv1alpha1.ImageListPullJobSpec{
					Images: []string{"nginx:1.9.1", "busybox:1.35"},
					ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
						CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Never},
					},
				},
			},
			ImagePullJobs: map[string]*appsv1alpha1.ImagePullJob{
				"nginx:1.9.1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j01",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "nginx:1.9.1",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
				"busybox:1.35": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j02",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "busybox:1.35",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
			},
			needToCreateImagePullJobs: []*appsv1alpha1.ImagePullJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    "default",
						GenerateName: "foo-",
						Labels:       make(map[string]string),
						Annotations:  make(map[string]string),
						OwnerReferences: []metav1.OwnerReference{
							*metav1.NewControllerRef(baseImageListPullJob, controllerKind),
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "nginx:1.9.1",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Never},
						},
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:    "default",
						GenerateName: "foo-",
						Labels:       make(map[string]string),
						Annotations:  make(map[string]string),
						OwnerReferences: []metav1.OwnerReference{
							*metav1.NewControllerRef(baseImageListPullJob, controllerKind),
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "busybox:1.35",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Never},
						},
					},
				},
			},
			needToDeleteImagePullJobs: []*appsv1alpha1.ImagePullJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j01",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "nginx:1.9.1",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "j02",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{UID: types.UID(jobUID), Name: "foo"},
						},
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						Image: "busybox:1.35",
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
						},
					}},
			},
		},
	}
	reconcileJob := ReconcileImageListPullJob{}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			needToCreateImagePullJobs, needToDeleteImagePullJobs := reconcileJob.computeImagePullJobActions(cs.ImageListPullJob, cs.ImagePullJobs, "v1")
			for _, job := range cs.needToCreateImagePullJobs {
				job.Labels[appsv1.ControllerRevisionHashLabelKey] = "v1"
			}
			assert.Equal(t, cs.needToCreateImagePullJobs, needToCreateImagePullJobs)
			assert.Equal(t, cs.needToDeleteImagePullJobs, needToDeleteImagePullJobs)
		})
	}
}

func createReconcileJob(scheme *k8sruntime.Scheme, initObjs ...client.Object) ReconcileImageListPullJob {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).
		WithIndex(&appsv1alpha1.ImagePullJob{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
			var owners []string
			for _, ref := range obj.GetOwnerReferences() {
				owners = append(owners, string(ref.UID))
			}
			return owners
		}).WithStatusSubresource(&appsv1alpha1.ImageListPullJob{}).Build()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "imagelistpulljob-controller"})
	reconcileJob := ReconcileImageListPullJob{
		Client:   fakeClient,
		scheme:   scheme,
		recorder: recorder,
		clock:    clock.RealClock{},
	}
	return reconcileJob
}
