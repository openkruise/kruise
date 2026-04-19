package uniteddeployment

import (
	"encoding/hex"
	"math/rand"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/apis/apps/v1beta1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/adapter"
)

func generateRandomId() string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		panic(err) // This shouldn't happen
	}
	return hex.EncodeToString(b)
}

func TestSubsetControl_convertToSubset(t *testing.T) {
	v1, v2 := "v1", "v2"
	selectorLabels := map[string]string{
		"foo": "bar",
	}
	subsetLabels := map[string]string{
		appsv1alpha1.ControllerRevisionHashLabelKey: v2,
		appsv1alpha1.SubSetNameLabelKey:             "subset-1",
	}
	getPod := func(revision string) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   generateRandomId(),
				Labels: selectorLabels,
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		pod = pod.DeepCopy()
		pod.Labels[appsv1alpha1.ControllerRevisionHashLabelKey] = revision
		return pod
	}
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	selector := &metav1.LabelSelector{
		MatchLabels: selectorLabels,
	}
	asts := &v1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "asts",
			Labels: subsetLabels,
		},
		Spec: v1beta1.StatefulSetSpec{
			Selector: selector,
			Replicas: ptr.To(int32(2)),
			UpdateStrategy: v1beta1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
		},
		Status: v1beta1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			ReadyReplicas:      1,
		},
	}
	oneIntStr := intstr.FromInt32(int32(1))
	cloneset := &appsv1beta1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cloneset",
			Labels: subsetLabels,
		},
		Spec: appsv1beta1.CloneSetSpec{
			Selector: selector,
			Replicas: ptr.To(int32(2)),
			UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateCloneSetUpdateStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateCloneSetStrategy{
					Partition: &oneIntStr,
				},
			},
		},
		Status: appsv1beta1.CloneSetStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			ReadyReplicas:      1,
		},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "rs",
			Labels: selectorLabels,
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: ptr.To(int32(2)),
			Selector: selector,
		},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "deploy",
			Labels: subsetLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: selector,
			Replicas: ptr.To(int32(2)),
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			ReadyReplicas:      1,
		},
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "sts",
			Labels: subsetLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: selector,
			Replicas: ptr.To(int32(2)),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			ReadyReplicas:      1,
		},
	}
	pod1, pod2 := getPod(v1), getPod(v2)
	fakeClient := fake.NewFakeClient(pod1, pod2, asts, cloneset, deploy, sts, rs)
	want := func(partition int32) *Subset {
		return &Subset{
			Spec: SubsetSpec{
				SubsetName: "subset-1",
				Replicas:   2,
				UpdateStrategy: SubsetUpdateStrategy{
					Partition: partition,
				},
			},
			Status: SubsetStatus{
				ObservedGeneration:   1,
				Replicas:             2,
				ReadyReplicas:        1,
				UpdatedReplicas:      1,
				UpdatedReadyReplicas: 1,
			},
		}
	}

	type fields struct {
		adapter adapter.Adapter
	}
	type args struct {
		set             metav1.Object
		updatedRevision string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *Subset
		wantErr bool
	}{
		{
			name: "asts",
			fields: fields{adapter: &adapter.AdvancedStatefulSetAdapter{
				Scheme: scheme,
				Client: fakeClient,
			}},
			args: args{asts, v2},
			want: want(1),
		}, {
			name: "cloneset",
			fields: fields{adapter: &adapter.CloneSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			}},
			args: args{cloneset, v2},
			want: want(1),
		}, {
			name: "deploy",
			fields: fields{adapter: &adapter.DeploymentAdapter{
				Client: fakeClient,
				Scheme: scheme,
			}},
			args: args{deploy, v2},
			want: want(0),
		}, {
			name: "sts",
			fields: fields{adapter: &adapter.StatefulSetAdapter{
				Client: fakeClient,
				Scheme: scheme,
			}},
			args: args{sts, v2},
			want: want(1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &SubsetControl{
				adapter: tt.fields.adapter,
			}
			tt.want.Status.UpdatedRevision = tt.args.updatedRevision
			got, err := m.convertToSubset(tt.args.set, tt.args.updatedRevision)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertToSubset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got.ObjectMeta = metav1.ObjectMeta{}
			if res := got.Spec.SubsetRef.Resources; len(res) != 1 || res[0].GetName() != tt.args.set.GetName() {
				t.Errorf("convertToSubset() subsetRef.Resources = %+v", res)
			}
			got.Spec.SubsetRef.Resources = nil
			if len(got.Spec.SubsetPods) != 2 {
				t.Errorf("convertToSubset() SubsetPods got = %+v, want %+v", got, tt.want)
			}
			got.Spec.SubsetPods = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertToSubset() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestUpdateSubset_MaxUnavailableClamp verifies that UpdateSubset never passes a negative
// MaxUnavailable value to the child workload.
//
// Background: when a subset is marked unschedulable and ReserveUnschedulablePods is enabled,
// the controller computes:
//
//	maxUnavailable = Spec.Replicas - ReadyReplicas + UpdateTimeoutPods
//
// During a scale-down race, ReadyReplicas can transiently exceed Spec.Replicas —
// pods from the receiving subset become Ready before the excess pods here terminate.
// Without a guard, this produces a negative int32 which disables all rolling-update
// limits on the child workload.
func TestUpdateSubset_MaxUnavailableClamp(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	selectorLabels := map[string]string{"app": "clamp-test"}

	makeUD := func() *appsv1alpha1.UnitedDeployment {
		return &appsv1alpha1.UnitedDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: "ud", Namespace: "default"},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
				Template: appsv1alpha1.SubsetTemplate{
					CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
						Spec: appsv1beta1.CloneSetSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{},
							},
						},
					},
				},
				Topology: appsv1alpha1.Topology{
					ScheduleStrategy: appsv1alpha1.UnitedDeploymentScheduleStrategy{
						Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
						Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
							ReserveUnschedulablePods: true,
						},
					},
					Subsets: []appsv1alpha1.Subset{{Name: "subset-a"}},
				},
			},
		}
	}

	tests := []struct {
		name               string
		specReplicas       int32
		readyReplicas      int32
		updateTimeoutPods  int32
		wantMaxUnavailable int32
	}{
		{
			name:               "positive — no clamp needed",
			specReplicas:       5,
			readyReplicas:      3,
			updateTimeoutPods:  1,
			wantMaxUnavailable: 3, // 5 - 3 + 1
		},
		{
			name:               "zero — exactly at capacity",
			specReplicas:       3,
			readyReplicas:      3,
			updateTimeoutPods:  0,
			wantMaxUnavailable: 0, // 3 - 3 + 0
		},
		{
			// THE BUG CASE: ReadyReplicas > Spec.Replicas (scale-down race).
			// Raw: 3 - 5 + 1 = -1 → must be clamped to 0.
			name:               "negative — scale-down race, clamped to 0",
			specReplicas:       3,
			readyReplicas:      5,
			updateTimeoutPods:  1,
			wantMaxUnavailable: 0,
		},
		{
			// UpdateTimeoutPods cannot compensate the large excess.
			// Raw: 2 - 10 + 3 = -5 → must be clamped to 0.
			name:               "large excess — clamped to 0",
			specReplicas:       2,
			readyReplicas:      10,
			updateTimeoutPods:  3,
			wantMaxUnavailable: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &appsv1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{Name: "subset-a-cs", Namespace: "default"},
				Spec: appsv1beta1.CloneSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
					Replicas: ptr.To(tt.specReplicas),
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs).Build()

			var capturedMaxUnavailable *int32
			spy := &spyMaxUnavailableAdapter{
				inner: &adapter.CloneSetAdapter{
					Client: fakeClient,
					Scheme: scheme,
				},
				onSetMaxUnavailable: func(v int32) {
					capturedMaxUnavailable = ptr.To(v)
				},
			}

			subset := &Subset{
				ObjectMeta: metav1.ObjectMeta{Name: "subset-a-cs", Namespace: "default"},
				Spec: SubsetSpec{
					SubsetName:   "subset-a",
					Replicas:     tt.specReplicas,
					SubsetRef:    ResourceRef{Resources: []metav1.Object{cs}},
				},
				Status: SubsetStatus{
					ReadyReplicas: tt.readyReplicas,
					UnschedulableStatus: SubsetUnschedulableStatus{
						Unschedulable:     true,
						UpdateTimeoutPods: tt.updateTimeoutPods,
					},
				},
			}

			m := &SubsetControl{Client: fakeClient, scheme: scheme, adapter: spy}

			if err := m.UpdateSubset(subset, makeUD(), "v1", tt.specReplicas, 0); err != nil {
				t.Fatalf("UpdateSubset returned unexpected error: %v", err)
			}
			if capturedMaxUnavailable == nil {
				t.Fatal("SetMaxUnavailable was never called")
			}
			got := *capturedMaxUnavailable
			if got < 0 {
				t.Errorf("MaxUnavailable must never be negative, got %d", got)
			}
			if got != tt.wantMaxUnavailable {
				t.Errorf("MaxUnavailable = %d, want %d", got, tt.wantMaxUnavailable)
			}
		})
	}
}

// spyMaxUnavailableAdapter wraps adapter.Adapter and intercepts SetMaxUnavailable.
type spyMaxUnavailableAdapter struct {
	inner               adapter.Adapter
	onSetMaxUnavailable func(val int32)
}

func (s *spyMaxUnavailableAdapter) SetMaxUnavailable(obj metav1.Object, val int32) metav1.Object {
	if s.onSetMaxUnavailable != nil {
		s.onSetMaxUnavailable(val)
	}
	return s.inner.SetMaxUnavailable(obj, val)
}
func (s *spyMaxUnavailableAdapter) NewResourceObject() client.Object { return s.inner.NewResourceObject() }
func (s *spyMaxUnavailableAdapter) NewResourceListObject() client.ObjectList {
	return s.inner.NewResourceListObject()
}
func (s *spyMaxUnavailableAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return s.inner.GetStatusObservedGeneration(obj)
}
func (s *spyMaxUnavailableAdapter) GetSubsetPods(obj metav1.Object) ([]*corev1.Pod, error) {
	return s.inner.GetSubsetPods(obj)
}
func (s *spyMaxUnavailableAdapter) GetSpecReplicas(obj metav1.Object) *int32 {
	return s.inner.GetSpecReplicas(obj)
}
func (s *spyMaxUnavailableAdapter) GetSpecPartition(obj metav1.Object, pods []*corev1.Pod) *int32 {
	return s.inner.GetSpecPartition(obj, pods)
}
func (s *spyMaxUnavailableAdapter) GetStatusReplicas(obj metav1.Object) int32 {
	return s.inner.GetStatusReplicas(obj)
}
func (s *spyMaxUnavailableAdapter) GetStatusReadyReplicas(obj metav1.Object) int32 {
	return s.inner.GetStatusReadyReplicas(obj)
}
func (s *spyMaxUnavailableAdapter) GetSubsetFailure() *string { return s.inner.GetSubsetFailure() }
func (s *spyMaxUnavailableAdapter) ApplySubsetTemplate(ud *appsv1alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, obj runtime.Object) error {
	return s.inner.ApplySubsetTemplate(ud, subsetName, revision, replicas, partition, obj)
}
func (s *spyMaxUnavailableAdapter) PostUpdate(ud *appsv1alpha1.UnitedDeployment, obj runtime.Object, revision string, partition int32) error {
	return s.inner.PostUpdate(ud, obj, revision, partition)
}
