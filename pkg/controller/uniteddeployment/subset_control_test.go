package uniteddeployment

import (
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"math/rand"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/adapter"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

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
	cloneset := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cloneset",
			Labels: subsetLabels,
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Selector: selector,
			Replicas: ptr.To(int32(2)),
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition: &oneIntStr,
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
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
