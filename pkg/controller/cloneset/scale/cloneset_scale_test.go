package scale

import (
	"context"
	"reflect"
	"sort"
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesettest "github.com/openkruise/kruise/pkg/controller/cloneset/test"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeControl() *realControl {
	return &realControl{
		Client:   fake.NewFakeClient(),
		recorder: record.NewFakeRecorder(10),
	}
}

func TestCreatePods(t *testing.T) {
	currentCS := clonesettest.NewCloneSet(3)
	updateCS := currentCS.DeepCopy()
	updateCS.Spec.Template.Spec.Containers[0].Env = []v1.EnvVar{{Name: "e-key", Value: "e-value"}}
	currentRevision := "revision-abc"
	updateRevision := "revision-xyz"

	ctrl := newFakeControl()
	created, err := ctrl.createPods(
		3,
		1,
		currentCS,
		updateCS,
		currentRevision,
		updateRevision,
		[]string{"id1", "id3", "id4"},
		sets.NewString("datadir-foo-id3"),
	)
	if err != nil {
		t.Fatalf("got unexpected error: %v", err)
	} else if !created {
		t.Fatalf("got unexpected created: %v", created)
	}

	pods := v1.PodList{}
	if err := ctrl.List(context.TODO(), &pods, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	expectedPods := []v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "default",
				Name:         "foo-id1",
				GenerateName: "foo-",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID:     "id1",
					apps.ControllerRevisionHashLabelKey: "revision-abc",
					"foo":                               "bar",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
				ResourceVersion: "1",
			},
			Spec: v1.PodSpec{
				ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				Containers: []v1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
						VolumeMounts: []v1.VolumeMount{
							{Name: "datadir", MountPath: "/tmp/data"},
							{Name: "home", MountPath: "/home"},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "datadir",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "datadir-foo-id1",
								ReadOnly:  false,
							},
						},
					},
					{
						Name: "home",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/tmp/home",
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "default",
				Name:         "foo-id3",
				GenerateName: "foo-",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID:     "id3",
					apps.ControllerRevisionHashLabelKey: "revision-xyz",
					"foo":                               "bar",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
				ResourceVersion: "1",
			},
			Spec: v1.PodSpec{
				ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				Containers: []v1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
						Env:   []v1.EnvVar{{Name: "e-key", Value: "e-value"}},
						VolumeMounts: []v1.VolumeMount{
							{Name: "datadir", MountPath: "/tmp/data"},
							{Name: "home", MountPath: "/home"},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "datadir",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "datadir-foo-id3",
								ReadOnly:  false,
							},
						},
					},
					{
						Name: "home",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/tmp/home",
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "default",
				Name:         "foo-id4",
				GenerateName: "foo-",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID:     "id4",
					apps.ControllerRevisionHashLabelKey: "revision-xyz",
					"foo":                               "bar",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
				ResourceVersion: "1",
			},
			Spec: v1.PodSpec{
				ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
				Containers: []v1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
						Env:   []v1.EnvVar{{Name: "e-key", Value: "e-value"}},
						VolumeMounts: []v1.VolumeMount{
							{Name: "datadir", MountPath: "/tmp/data"},
							{Name: "home", MountPath: "/home"},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "datadir",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "datadir-foo-id4",
								ReadOnly:  false,
							},
						},
					},
					{
						Name: "home",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/tmp/home",
							},
						},
					},
				},
			},
		},
	}

	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	if len(pods.Items) != len(expectedPods) {
		t.Fatalf("expected pods \n%s\ngot pods\n%s", util.DumpJSON(expectedPods), util.DumpJSON(pods.Items))
	}

	for i := range expectedPods {
		appsv1alpha1.SetDefaultPod(&expectedPods[i])
		if v, ok := pods.Items[i].Annotations[appspub.LifecycleTimestampKey]; ok {
			if expectedPods[i].Annotations == nil {
				expectedPods[i].Annotations = make(map[string]string)
			}
			expectedPods[i].Annotations[appspub.LifecycleTimestampKey] = v
		}
	}

	if !reflect.DeepEqual(expectedPods, pods.Items) {
		t.Fatalf("expected pods \n%s\ngot pods\n%s", util.DumpJSON(expectedPods), util.DumpJSON(pods.Items))
	}

	pvcs := v1.PersistentVolumeClaimList{}
	if err := ctrl.List(context.TODO(), &pvcs, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list pvcs: %v", err)
	}
	expectedPVCs := []v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "datadir-foo-id1",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id1",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
				ResourceVersion: "1",
			},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("10"),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "datadir-foo-id4",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id4",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
				ResourceVersion: "1",
			},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("10"),
					},
				},
			},
		},
	}
	for i := range expectedPVCs {
		corev1.SetDefaults_PersistentVolumeClaim(&expectedPVCs[i])
	}
	sort.Slice(pvcs.Items, func(i, j int) bool { return pvcs.Items[i].Name < pvcs.Items[j].Name })
	if !reflect.DeepEqual(expectedPVCs, pvcs.Items) {
		t.Fatalf("expected pvcs \n%s\ngot pvcs\n%s", util.DumpJSON(expectedPVCs), util.DumpJSON(pvcs.Items))
	}

	exp := clonesetutils.ScaleExpectations.GetExpectations("default/foo")
	expectedExp := map[expectations.ScaleAction]sets.String{
		expectations.Create: sets.NewString("foo-id1", "foo-id3", "foo-id4", "datadir-foo-id1", "datadir-foo-id4"),
	}
	if !reflect.DeepEqual(expectedExp, exp) {
		t.Fatalf("expected expectations: %v, got %v", expectedExp, exp)
	}
}

func TestDeletePods(t *testing.T) {
	cs := &appsv1alpha1.CloneSet{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "foo"}}
	podsToDelete := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "default",
				Name:         "foo-id1",
				GenerateName: "foo-",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id1",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "default",
				Name:         "foo-id3",
				GenerateName: "foo-",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id3",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
			},
		},
	}

	pvcs := []*v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "datadir-foo-id1",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id1",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "datadir-foo-id2",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id2",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "datadir-foo-id3",
				Labels: map[string]string{
					appsv1alpha1.CloneSetInstanceID: "id3",
					"foo":                           "bar",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "apps.kruise.io/v1alpha1",
						Kind:               "CloneSet",
						Name:               "foo",
						UID:                "test",
						Controller:         func() *bool { a := true; return &a }(),
						BlockOwnerDeletion: func() *bool { a := true; return &a }(),
					},
				},
			},
		},
	}

	ctrl := newFakeControl()
	for _, p := range podsToDelete {
		_ = ctrl.Create(context.TODO(), p)
	}
	for _, p := range pvcs {
		_ = ctrl.Create(context.TODO(), p)
	}

	deleted, err := ctrl.deletePods(cs, podsToDelete, pvcs)
	if err != nil {
		t.Fatalf("failed to delete got pods: %v", err)
	} else if !deleted {
		t.Fatalf("failed to delete got pods: not deleted")
	}

	gotPods := v1.PodList{}
	if err := ctrl.List(context.TODO(), &gotPods, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(gotPods.Items) > 0 {
		t.Fatalf("expected no pods left, actually: %v", gotPods.Items)
	}

	gotPVCs := v1.PersistentVolumeClaimList{}
	if err := ctrl.List(context.TODO(), &gotPVCs, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list pvcs: %v", err)
	}
	if len(gotPVCs.Items) != 1 || reflect.DeepEqual(gotPVCs.Items[0], pvcs[1]) {
		t.Fatalf("unexpected pvcs: %v", util.DumpJSON(gotPVCs.Items))
	}
}
