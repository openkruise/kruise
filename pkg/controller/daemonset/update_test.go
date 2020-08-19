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

package daemonset

import (
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	utilpointer "k8s.io/utils/pointer"
)

func Test_maxRevision(t *testing.T) {
	type args struct {
		histories []*apps.ControllerRevision
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "GetMaxRevision",
			args: args{
				histories: []*apps.ControllerRevision{
					{
						Revision: 123456789,
					},
					{
						Revision: 213456789,
					},
					{
						Revision: 312456789,
					},
				},
			},
			want: 312456789,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxRevision(tt.args.histories); got != tt.want {
				t.Errorf("maxRevision() = %v, want %v", got, tt.want)
			}
			t.Logf("maxRevision() = %v", tt.want)
		})
	}
}

func TestGetTemplateGeneration(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	constNum := int64(1000)
	tests := []struct {
		name    string
		args    args
		want    *int64
		wantErr bool
	}{
		{
			name: "GetTemplateGeneration",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							apps.DeprecatedTemplateGeneration: "1000",
						},
					},
					Spec:   appsv1alpha1.DaemonSetSpec{},
					Status: appsv1alpha1.DaemonSetStatus{},
				},
			},
			want:    &constNum,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTemplateGeneration(tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTemplateGeneration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if *got != *tt.want {
				t.Errorf("GetTemplateGeneration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterDaemonPodsNodeToUpdate(t *testing.T) {
	now := metav1.Now()
	type testcase struct {
		name             string
		rolling          *appsv1alpha1.RollingUpdateDaemonSet
		hash             string
		nodeToDaemonPods map[string][]*corev1.Pod
		nodes            []*corev1.Node
		expectNodes      []string
	}

	tests := []testcase{
		{
			name: "Standard,partition=0",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(0),
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			expectNodes: []string{"n2", "n3", "n1"},
		},
		{
			name: "Standard,partition=1",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(1),
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			expectNodes: []string{"n1", "n3"},
		},
		{
			name: "Standard,partition=1,selector=1",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(1),
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "canary"}},
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n4": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			nodes: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n4"}},
			},
			expectNodes: []string{"n2", "n3"},
		},
		{
			name: "Standard,partition=2,selector=3",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(2),
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "canary"}},
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n4": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			nodes: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n4", Labels: map[string]string{"node-type": "canary"}}},
			},
			expectNodes: []string{"n2", "n4"},
		},
		{
			name: "Standard,partition=0,selector=3,terminating",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(0),
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "canary"}},
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}, DeletionTimestamp: &now}},
				},
				"n4": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			nodes: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n4"}},
			},
			expectNodes: []string{"n3", "n2", "n1"},
		},
	}

	testFn := func(test *testcase, t *testing.T) {
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		for _, node := range test.nodes {
			if err := indexer.Add(node); err != nil {
				t.Fatalf("failed to add node into indexer: %v", err)
			}
		}
		nodeLister := corelisters.NewNodeLister(indexer)
		dsc := &ReconcileDaemonSet{nodeLister: nodeLister}

		ds := &appsv1alpha1.DaemonSet{Spec: appsv1alpha1.DaemonSetSpec{UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
			Type:          appsv1alpha1.RollingUpdateDaemonSetStrategyType,
			RollingUpdate: test.rolling,
		}}}
		got, err := dsc.filterDaemonPodsNodeToUpdate(ds, test.hash, test.nodeToDaemonPods)
		if err != nil {
			t.Fatalf("failed to call filterDaemonPodsNodeToUpdate: %v", err)
		}
		if !reflect.DeepEqual(got, test.expectNodes) {
			t.Fatalf("expected %v, got %v", test.expectNodes, got)
		}
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			testFn(&tests[i], t)
		})
	}
}
