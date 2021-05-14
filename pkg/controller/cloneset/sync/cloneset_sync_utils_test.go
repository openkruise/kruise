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

package sync

import (
	"fmt"
	"reflect"
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCalculateDiffsWithExpectation(t *testing.T) {
	oldRevision := "old_rev"
	newRevision := "new_rev"

	cases := []struct {
		name               string
		set                *appsv1alpha1.CloneSet
		pods               []*v1.Pod
		revisionConsistent bool
		disableFeatureGate bool
		expectResult       expectationDiffs
	}{
		{
			name:         "an empty cloneset",
			set:          createTestCloneSet(0, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods:         []*v1.Pod{},
			expectResult: expectationDiffs{},
		},
		{
			name:         "increase replicas to 5 (step 1/2)",
			set:          createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods:         []*v1.Pod{},
			expectResult: expectationDiffs{scaleNum: 5},
		},
		{
			name: "increase replicas to 5 (step 2/2)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "specified delete 1 pod (all ready) (step 1/3)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{deleteReadyLimit: 1},
		},
		{
			name: "specified delete 1 pod (all ready) (step 2/3)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1},
		},
		{
			name: "specified delete 1 pod (all ready) (step 3/3)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "specified delete 2 pod (all ready) (step 1/6)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{deleteReadyLimit: 1},
		},
		{
			name: "specified delete 2 pod (all ready) (step 2/6)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1},
		},
		{
			name: "specified delete 2 pod (all ready) (step 3/6)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "specified delete 2 pod (all ready) (step 4/6)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false), // new creation
			},
			expectResult: expectationDiffs{deleteReadyLimit: 1},
		},
		{
			name: "specified delete 2 pod (all ready) (step 5/6)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1},
		},
		{
			name: "specified delete 2 pod (all ready) (step 6/6)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "specified delete 2 pod and replicas to 4 (step 1/3)",
			set:  createTestCloneSet(4, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: -1, deleteReadyLimit: 2},
		},
		{
			name: "specified delete 2 pod and replicas to 4 (step 2/3)",
			set:  createTestCloneSet(4, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1},
		},
		{
			name: "specified delete 2 pod and replicas to 4 (step 3/3)",
			set:  createTestCloneSet(4, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "update partition=3 (step 1/3)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{updateNum: 2, updateMaxUnavailable: 1},
		},
		{
			name: "update partition=3 (step 2/3)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{updateNum: 1, updateMaxUnavailable: 1},
		},
		{
			name: "update partition=3 (step 3/3)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "rollback partition=4 (step 1/3)",
			set:  createTestCloneSet(5, intstr.FromInt(4), intstr.FromInt(2), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{updateNum: -3, updateMaxUnavailable: 2},
		},
		{
			name: "rollback partition=4 (step 2/3)",
			set:  createTestCloneSet(5, intstr.FromInt(4), intstr.FromInt(2), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{updateNum: -1, updateMaxUnavailable: 2},
		},
		{
			name: "rollback partition=4 (step 3/3)",
			set:  createTestCloneSet(5, intstr.FromInt(4), intstr.FromInt(2), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "specified delete with maxSurge (step 1/4)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1},
		},
		{
			name: "specified delete with maxSurge (step 2/4)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{useSurge: 1},
		},
		{
			name: "specified delete with maxSurge (step 3/4)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{deleteReadyLimit: 1, useSurge: 1},
		},
		{
			name: "specified delete with maxSurge (step 4/4)",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "update in-place partition=3 with maxSurge (step 1/4)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 2, updateMaxUnavailable: 1},
		},
		{
			name: "update in-place partition=3 with maxSurge (step 2/4)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{useSurge: 1, updateNum: 1, updateMaxUnavailable: 2},
		},
		{
			name: "update in-place partition=3 with maxSurge (step 3/4)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateUpdating, false, false), // new in-place update
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),   // new creation
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 0},
		},
		{
			name: "update in-place partition=3 with maxSurge (step 4/4)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),  // new in-place update
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 1},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 1/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 2, updateMaxUnavailable: 1},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 2/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{useSurge: 1, updateNum: 1, updateMaxUnavailable: 2},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 3/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, true),   // begin to recreate
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{useSurge: 1, useSurgeOldRevision: 1, deleteReadyLimit: 1, updateNum: 1, updateMaxUnavailable: 2},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 4/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{useSurge: 1, scaleNum: 1, updateNum: 1, updateMaxUnavailable: 1},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 5/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation for update
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 0},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 6/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),  // new creation
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation for update
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 1},
		},
		{
			name: "update recreate partition=3 with maxSurge (step 7/7)",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false), // new creation
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false), // new creation for update
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "revision consistent 1",
			set:  createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			revisionConsistent: true,
			expectResult:       expectationDiffs{},
		},
		{
			name: "revision consistent 2",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			revisionConsistent: true,
			expectResult:       expectationDiffs{},
		},
		{
			name: "revision consistent 3",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			revisionConsistent: true,
			expectResult:       expectationDiffs{},
		},
		{
			name: "revision consistent 4",
			set:  createTestCloneSet(5, intstr.FromInt(1), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			revisionConsistent: true,
			expectResult:       expectationDiffs{updateNum: 2, updateMaxUnavailable: 1},
		},
		{
			name: "allow to update when fail to scale out normally",
			set:  createTestCloneSet(5, intstr.FromInt(1), intstr.FromInt(2), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			revisionConsistent: true,
			expectResult:       expectationDiffs{scaleNum: 1, updateNum: 1, updateMaxUnavailable: 1},
		},
		{
			name: "allow to update when fail to scale in normally",
			set:  createTestCloneSet(5, intstr.FromInt(1), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			revisionConsistent: true,
			expectResult:       expectationDiffs{scaleNum: -1, scaleNumOldRevision: -2, deleteReadyLimit: 2, updateNum: 1, updateMaxUnavailable: 2},
		},
		{
			name: "disable rollback feature-gate",
			set:  createTestCloneSet(5, intstr.FromInt(4), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			disableFeatureGate: true,
			expectResult:       expectationDiffs{},
		},
	}

	for i := range cases {
		t.Run(cases[i].name, func(t *testing.T) {
			if cases[i].disableFeatureGate {
				_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.CloneSetPartitionRollback))
			} else {
				_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.CloneSetPartitionRollback))
			}
			current := oldRevision
			if cases[i].revisionConsistent {
				current = newRevision
			}
			res := calculateDiffsWithExpectation(cases[i].set, cases[i].pods, current, newRevision)
			if !reflect.DeepEqual(res, cases[i].expectResult) {
				t.Errorf("got %#v, expect %#v", res, cases[i].expectResult)
			}
		})
	}
}

func createTestCloneSet(replicas int32, partition, maxUnavailable, maxSurge intstr.IntOrString) *appsv1alpha1.CloneSet {
	return &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: &replicas,
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      &partition,
				MaxSurge:       &maxSurge,
				MaxUnavailable: &maxUnavailable,
			},
		},
	}
}

func createTestPod(revisionHash string, lifecycleState appspub.LifecycleStateType, ready bool, specifiedDelete bool) *v1.Pod {
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		apps.ControllerRevisionHashLabelKey: revisionHash,
		appspub.LifecycleStateKey:           string(lifecycleState),
	}}}
	if ready {
		pod.Status = v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}}
	}
	if specifiedDelete {
		pod.Labels[appsv1alpha1.SpecifiedDeleteKey] = "true"
	}
	return pod
}

func TestSortingActivePodsWithDeletionCost(t *testing.T) {
	now := metav1.Now()
	then := metav1.Time{Time: now.AddDate(0, -1, 0)}
	zeroTime := metav1.Time{}
	pod := func(podName, nodeName string, phase v1.PodPhase, ready bool, restarts int32, readySince metav1.Time, created metav1.Time, annotations map[string]string) *v1.Pod {
		var conditions []v1.PodCondition
		var containerStatuses []v1.ContainerStatus
		if ready {
			conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: readySince}}
			containerStatuses = []v1.ContainerStatus{{RestartCount: restarts}}
		}
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: created,
				Name:              podName,
				Annotations:       annotations,
			},
			Spec: v1.PodSpec{NodeName: nodeName},
			Status: v1.PodStatus{
				Conditions:        conditions,
				ContainerStatuses: containerStatuses,
				Phase:             phase,
			},
		}
	}
	var (
		unscheduledPod                      = pod("unscheduled", "", v1.PodPending, false, 0, zeroTime, zeroTime, nil)
		scheduledPendingPod                 = pod("pending", "node", v1.PodPending, false, 0, zeroTime, zeroTime, nil)
		unknownPhasePod                     = pod("unknown-phase", "node", v1.PodUnknown, false, 0, zeroTime, zeroTime, nil)
		runningNotReadyPod                  = pod("not-ready", "node", v1.PodRunning, false, 0, zeroTime, zeroTime, nil)
		runningReadyNoLastTransitionTimePod = pod("ready-no-last-transition-time", "node", v1.PodRunning, true, 0, zeroTime, zeroTime, nil)
		runningReadyNow                     = pod("ready-now", "node", v1.PodRunning, true, 0, now, now, nil)
		runningReadyThen                    = pod("ready-then", "node", v1.PodRunning, true, 0, then, then, nil)
		runningReadyNowHighRestarts         = pod("ready-high-restarts", "node", v1.PodRunning, true, 9001, now, now, nil)
		runningReadyNowCreatedThen          = pod("ready-now-created-then", "node", v1.PodRunning, true, 0, now, then, nil)
		lowPodDeletionCost                  = pod("low-deletion-cost", "node", v1.PodRunning, true, 0, now, then, map[string]string{PodDeletionCost: "10"})
		highPodDeletionCost                 = pod("high-deletion-cost", "node", v1.PodRunning, true, 0, now, then, map[string]string{PodDeletionCost: "100"})
	)
	equalityTests := []*v1.Pod{
		unscheduledPod,
		scheduledPendingPod,
		unknownPhasePod,
		runningNotReadyPod,
		runningReadyNowCreatedThen,
		runningReadyNow,
		runningReadyThen,
		runningReadyNowHighRestarts,
		runningReadyNowCreatedThen,
	}
	for _, pod := range equalityTests {
		podsWithDeletionCost := ActivePodsWithDeletionCost([]*v1.Pod{pod, pod})
		if podsWithDeletionCost.Less(0, 1) || podsWithDeletionCost.Less(1, 0) {
			t.Errorf("expected pod %q not to be less than than itself", pod.Name)
		}
	}
	type podWithDeletionCost *v1.Pod
	inequalityTests := []struct {
		lesser, greater podWithDeletionCost
	}{
		{lesser: podWithDeletionCost(unscheduledPod), greater: podWithDeletionCost(scheduledPendingPod)},
		{lesser: podWithDeletionCost(unscheduledPod), greater: podWithDeletionCost(scheduledPendingPod)},
		{lesser: podWithDeletionCost(scheduledPendingPod), greater: podWithDeletionCost(unknownPhasePod)},
		{lesser: podWithDeletionCost(unknownPhasePod), greater: podWithDeletionCost(runningNotReadyPod)},
		{lesser: podWithDeletionCost(runningNotReadyPod), greater: podWithDeletionCost(runningReadyNoLastTransitionTimePod)},
		{lesser: podWithDeletionCost(runningReadyNoLastTransitionTimePod), greater: podWithDeletionCost(runningReadyNow)},
		{lesser: podWithDeletionCost(runningReadyNow), greater: podWithDeletionCost(runningReadyThen)},
		{lesser: podWithDeletionCost(runningReadyNowHighRestarts), greater: podWithDeletionCost(runningReadyNow)},
		{lesser: podWithDeletionCost(runningReadyNow), greater: podWithDeletionCost(runningReadyNowCreatedThen)},
		{lesser: podWithDeletionCost(lowPodDeletionCost), greater: podWithDeletionCost(highPodDeletionCost)},
	}
	for i, test := range inequalityTests {
		t.Run(fmt.Sprintf("test%d", i), func(t *testing.T) {

			podsWithDeletionCost := ActivePodsWithDeletionCost([]*v1.Pod{test.lesser, test.greater})
			if !podsWithDeletionCost.Less(0, 1) {
				t.Errorf("expected pod %q to be less than %q", podsWithDeletionCost[0].Name, podsWithDeletionCost[1].Name)
			}
			if podsWithDeletionCost.Less(1, 0) {
				t.Errorf("expected pod %q not to be less than %v", podsWithDeletionCost[1].Name, podsWithDeletionCost[0].Name)
			}
		})
	}
}
