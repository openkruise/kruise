package daemonset

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_nodeInSameCondition(t *testing.T) {
	type args struct {
		old []corev1.NodeCondition
		cur []corev1.NodeCondition
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nodeInSameCondition",
			args: args{
				old: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
				},
				cur: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
				},
			},
			want: true,
		},
		{
			name: "nodeInSameCondition2",
			args: args{
				old: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
				},
				cur: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "test-2",
						Status: corev1.ConditionFalse,
					},
				},
			},
			want: true,
		},
		{
			name: "nodeNotInSameCondition",
			args: args{
				old: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "test-3",
						Status: corev1.ConditionTrue,
					},
				},
				cur: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "test-2",
						Status: corev1.ConditionFalse,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeInSameCondition(tt.args.old, tt.args.cur); got != tt.want {
				t.Errorf("nodeInSameCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldIgnoreNodeUpdate(t *testing.T) {
	type args struct {
		oldNode corev1.Node
		curNode corev1.Node
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "ShouldIgnoreNodeUpdate",
			args: args{
				oldNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1111",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{},
					},
				},
				curNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1111",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{},
					},
				},
			},
			want: true,
		},
		{
			name: "ShouldNotIgnoreNodeUpdate",
			args: args{
				oldNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1111",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   "test-1",
								Status: corev1.ConditionTrue,
							},
							{
								Type:   "test-3",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				curNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1112",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   "test-1",
								Status: corev1.ConditionTrue,
							},
							{
								Type:   "test-2",
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldIgnoreNodeUpdate(tt.args.oldNode, tt.args.curNode); got != tt.want {
				t.Errorf("ShouldIgnoreNodeUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getBurstReplicas(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "getBurstReplicas",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					Spec: appsv1alpha1.DaemonSetSpec{
						BurstReplicas: &intstr.IntOrString{IntVal: 10},
					},
				},
			},
			want: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getBurstReplicas(tt.args.ds); got != tt.want {
				t.Errorf("getBurstReplicas() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPodRevision(t *testing.T) {
	type args struct {
		pod metav1.Object
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "GetPodRevision",
			args: args{
				pod: &corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey: "111222333",
						},
					},
					Spec:   corev1.PodSpec{},
					Status: corev1.PodStatus{},
				},
			},
			want: "111222333",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPodRevision(tt.args.pod); got != tt.want {
				t.Errorf("GetPodRevision() = %v, want %v", got, tt.want)
			}
		})
	}
}
