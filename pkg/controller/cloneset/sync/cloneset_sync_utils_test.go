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
			expectResult: expectationDiffs{scaleNum: 5, scaleUpLimit: 5},
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
			expectResult: expectationDiffs{scaleNum: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{scaleNum: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{scaleNum: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{scaleNum: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 2, updateMaxUnavailable: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 2, updateMaxUnavailable: 1, scaleUpLimit: 1},
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
			expectResult: expectationDiffs{useSurge: 1, scaleNum: 1, updateNum: 1, updateMaxUnavailable: 1, scaleUpLimit: 1},
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
			name: "update recreate partition=99% with maxUnavailable=3, maxSurge=2 (step 1/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromInt(3), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 1, updateMaxUnavailable: 3, scaleUpLimit: 1},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=3, maxSurge=2 (step 2/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromInt(3), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 3},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=3, maxSurge=2 (step 3/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromInt(3), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=40%, maxSurge=30% (step 1/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromString("40%"), intstr.FromString("30%")),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 1, updateMaxUnavailable: 2, scaleUpLimit: 1},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=40%, maxSurge=30% (step 2/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromString("40%"), intstr.FromString("30%")),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 2},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=40%, maxSurge=30% (step 3/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromString("40%"), intstr.FromString("30%")),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=30%, maxSurge=30% (step 1/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromString("30%"), intstr.FromString("30%")),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{scaleNum: 1, useSurge: 1, updateNum: 1, updateMaxUnavailable: 1, scaleUpLimit: 1},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=30%, maxSurge=30% (step 2/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromString("30%"), intstr.FromString("30%")),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{scaleNum: -1, scaleNumOldRevision: -1, deleteReadyLimit: 1},
		},
		{
			name: "update recreate partition=99% with maxUnavailable=30%, maxSurge=30% (step 3/3)",
			set:  createTestCloneSet(5, intstr.FromString("99%"), intstr.FromString("30%"), intstr.FromString("30%")),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
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
			expectResult:       expectationDiffs{scaleNum: 1, updateNum: 1, updateMaxUnavailable: 1, scaleUpLimit: 1},
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
		{
			name:         "increase replicas 0 to 5 with scale maxUnavailable = 2",
			set:          setScaleStrategy(createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)), intstr.FromInt(2)),
			pods:         []*v1.Pod{},
			expectResult: expectationDiffs{scaleNum: 5, scaleUpLimit: 2},
		},
		{
			name: "increase replicas 3 to 6 with scale maxUnavailable = 2, not ready pod = 1",
			set:  setScaleStrategy(createTestCloneSet(6, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{scaleNum: 3, scaleUpLimit: 1},
		},
		{
			name: "increase replicas 3 to 6 with scale maxUnavailable = 2, not ready pod = 2",
			set:  setScaleStrategy(createTestCloneSet(6, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{scaleNum: 3, scaleUpLimit: 0},
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

func setScaleStrategy(cs *appsv1alpha1.CloneSet, maxUnavailable intstr.IntOrString) *appsv1alpha1.CloneSet {
	cs.Spec.ScaleStrategy = appsv1alpha1.CloneSetScaleStrategy{
		MaxUnavailable: &maxUnavailable,
	}
	return cs
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
