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
	"math/rand"
	"reflect"
	"sort"
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
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
	}

	for i := range cases {
		t.Run(cases[i].name, func(t *testing.T) {
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

// create count pods with the given phase for the given rc (same selectors and namespace), and add them to the store.
func newPodList(count int, status v1.PodPhase) *v1.PodList {
	var pods []v1.Pod
	for i := 0; i < count; i++ {
		newPod := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%d", i),
				Labels:    map[string]string{"foo": "bar"},
				Namespace: metav1.NamespaceDefault,
			},
			Status: v1.PodStatus{Phase: status},
		}

		pods = append(pods, newPod)
	}
	return &v1.PodList{
		Items: pods,
	}
}

func TestSortingActiveAndPriorityPodsLabelMiss(t *testing.T) {
	numPods := 9
	podList := newPodList(numPods, v1.PodRunning)

	pods := make([]*v1.Pod, len(podList.Items))
	for i := range podList.Items {
		pods[i] = &podList.Items[i]
	}
	// pods[0] is not scheduled yet.
	pods[0].Spec.NodeName = ""
	pods[0].Status.Phase = v1.PodPending
	// pods[1] is scheduled but pending.
	pods[1].Spec.NodeName = "bar"
	pods[1].Status.Phase = v1.PodPending
	// pods[2] is unknown.
	pods[2].Spec.NodeName = "foo"
	pods[2].Status.Phase = v1.PodUnknown
	// pods[3] is running but not ready.
	pods[3].Spec.NodeName = "foo"
	pods[3].Status.Phase = v1.PodRunning
	// pods[4] is running and ready but without LastTransitionTime.
	now := metav1.Now()
	pods[4].Spec.NodeName = "foo"
	pods[4].Status.Phase = v1.PodRunning
	pods[4].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}
	pods[4].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 3}, {RestartCount: 0}}
	// pods[5] is running and ready and with LastTransitionTime.
	pods[5].Spec.NodeName = "foo"
	pods[5].Status.Phase = v1.PodRunning
	pods[5].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: now}}
	pods[5].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 3}, {RestartCount: 0}}
	// pods[6] is running ready for a longer time than pods[5].
	then := metav1.Time{Time: now.AddDate(0, -1, 0)}
	pods[6].Spec.NodeName = "foo"
	pods[6].Status.Phase = v1.PodRunning
	pods[6].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[6].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 3}, {RestartCount: 0}}
	// pods[7] has lower container restart count than pods[6].
	pods[7].Spec.NodeName = "foo"
	pods[7].Status.Phase = v1.PodRunning
	pods[7].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[7].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 2}, {RestartCount: 1}}
	pods[7].CreationTimestamp = now
	// pods[8] is older than pods[7].
	pods[8].Spec.NodeName = "foo"
	pods[8].Status.Phase = v1.PodRunning
	pods[8].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[8].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 2}, {RestartCount: 1}}
	pods[8].CreationTimestamp = then

	priority := []appsv1alpha1.CloneSetDeletePriorityWeightTerm{
		{
			Weight: 1,
			MatchSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "value"},
			},
		},
	}

	getOrder := func(pods []*v1.Pod) []string {
		names := make([]string, len(pods))
		for i := range pods {
			names[i] = pods[i].Name
		}
		return names
	}

	expected := getOrder(pods)

	for i := 0; i < 20; i++ {
		idx := rand.Perm(numPods)
		randomizedPods := make([]*v1.Pod, numPods)
		for j := 0; j < numPods; j++ {
			randomizedPods[j] = pods[idx[j]]
		}
		sort.Sort(ActivePodsWithPriority{randomizedPods, deletePrioritySelector(priority)})
		actual := getOrder(randomizedPods)

		assert.EqualValues(t, expected, actual, "expected %v, got %v", expected, actual)
	}
}

func TestSortingActiveAndPriorityPods(t *testing.T) {
	numPods := 10
	podList := newPodList(numPods, v1.PodRunning)

	pods := make([]*v1.Pod, len(podList.Items))
	for i := range podList.Items {
		pods[i] = &podList.Items[i]
	}
	// pods[0] is not scheduled yet.
	pods[0].Spec.NodeName = ""
	pods[0].Status.Phase = v1.PodPending
	// pods[1] by label.
	pods[1].Spec.NodeName = "foo"
	pods[1].ObjectMeta.Labels = map[string]string{"key": "a", "key1": "value1"}
	pods[1].Status.Phase = v1.PodRunning
	// pods[2] by label.
	pods[2].Spec.NodeName = "bar"
	pods[2].ObjectMeta.Labels = map[string]string{"key": "c"}
	pods[2].Status.Phase = v1.PodPending
	// pods[3] by label.
	pods[3].Spec.NodeName = "foo"
	pods[3].ObjectMeta.Labels = map[string]string{"key": "c"}
	pods[3].Status.Phase = v1.PodUnknown
	// pods[4] by label.
	pods[4].Spec.NodeName = "foo"
	pods[4].ObjectMeta.Labels = map[string]string{"key": "d"}
	pods[4].Status.Phase = v1.PodUnknown
	// pods[5] label miss.
	pods[5].Spec.NodeName = "foo"
	pods[5].Status.Phase = v1.PodPending
	// pods[6] CreationTimestamp late and label first in order.
	now := metav1.Now()
	then := metav1.Time{Time: now.AddDate(0, -1, 0)}
	pods[6].Spec.NodeName = "foo"
	pods[6].Status.Phase = v1.PodRunning
	pods[6].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[6].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 3}, {RestartCount: 0}}
	pods[6].CreationTimestamp = now
	pods[6].ObjectMeta.Labels = map[string]string{"key": "a", "key1": "value1"}
	// pods[7] is older than pods[6].
	pods[7].Spec.NodeName = "foo"
	pods[7].Status.Phase = v1.PodRunning
	pods[7].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[7].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 2}, {RestartCount: 1}}
	pods[7].CreationTimestamp = then
	pods[7].ObjectMeta.Labels = map[string]string{"key": "a", "key1": "value1"}
	// pods[8] by label.
	pods[8].Spec.NodeName = "foo"
	pods[8].Status.Phase = v1.PodRunning
	pods[8].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[8].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 2}, {RestartCount: 1}}
	pods[8].CreationTimestamp = now
	pods[8].ObjectMeta.Labels = map[string]string{"key": "c", "key1": "value1"}
	// pods[8] label miss.
	pods[9].Spec.NodeName = "foo"
	pods[9].Status.Phase = v1.PodRunning
	pods[9].Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: then}}
	pods[9].Status.ContainerStatuses = []v1.ContainerStatus{{RestartCount: 2}, {RestartCount: 1}}
	pods[9].CreationTimestamp = now

	priority := []appsv1alpha1.CloneSetDeletePriorityWeightTerm{
		{
			Weight: 50,
			MatchSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "c"},
			},
		}, {
			Weight: 99,
			MatchSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "a"},
			},
		}, {
			Weight: 50,
			MatchSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "d"},
			},
		},
	}

	getOrder := func(pods []*v1.Pod) []string {
		names := make([]string, len(pods))
		for i := range pods {
			names[i] = pods[i].Name
		}
		return names
	}

	expected := getOrder(pods)

	for i := 0; i < 20; i++ {
		idx := rand.Perm(numPods)
		randomizedPods := make([]*v1.Pod, numPods)
		for j := 0; j < numPods; j++ {
			randomizedPods[j] = pods[idx[j]]
		}
		sort.Sort(ActivePodsWithPriority{randomizedPods, deletePrioritySelector(priority)})
		actual := getOrder(randomizedPods)

		assert.EqualValues(t, expected, actual, "expected %v, got %v", expected, actual)
	}
}
