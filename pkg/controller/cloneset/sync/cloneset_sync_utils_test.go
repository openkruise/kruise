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
	"flag"
	"fmt"
	"reflect"
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/revision"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

func TestCalculateDiffsWithExpectation(t *testing.T) {
	oldRevision := "old_rev"
	newRevision := "new_rev"

	cases := []struct {
		name               string
		set                *appsv1alpha1.CloneSet
		setLabels          map[string]string
		pods               []*v1.Pod
		revisionConsistent bool
		disableFeatureGate bool
		isPodUpdate        IsPodUpdateFunc
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
			expectResult: expectationDiffs{ScaleUpNum: 5, ScaleUpLimit: 5},
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
			expectResult: expectationDiffs{DeleteReadyLimit: 1},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{DeleteReadyLimit: 1},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{DeleteReadyLimit: 1},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{DeleteReadyLimit: 2},
		},
		{
			name: "specified delete 2 pod and replicas to 4 (step 2/3)",
			set:  createTestCloneSet(4, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{UpdateNum: 2, UpdateMaxUnavailable: 1},
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
			expectResult: expectationDiffs{UpdateNum: 1, UpdateMaxUnavailable: 1},
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
			expectResult: expectationDiffs{UpdateNum: -3, UpdateMaxUnavailable: 2},
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
			expectResult: expectationDiffs{UpdateNum: -1, UpdateMaxUnavailable: 2},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, UseSurge: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{UseSurge: 1},
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
			expectResult: expectationDiffs{DeleteReadyLimit: 1, UseSurge: 1},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, UseSurge: 1, UpdateNum: 2, UpdateMaxUnavailable: 1, ScaleUpLimit: 1},
		},
		{
			name: "update in-place partition=3 with maxSurge step 2/4",
			set:  createTestCloneSet(5, intstr.FromInt(3), intstr.FromInt(1), intstr.FromInt(1)),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false), // new creation
			},
			expectResult: expectationDiffs{UseSurge: 1, UpdateNum: 1, UpdateMaxUnavailable: 2},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 0},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 1},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, UseSurge: 1, UpdateNum: 2, UpdateMaxUnavailable: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{UseSurge: 1, UpdateNum: 1, UpdateMaxUnavailable: 2},
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
			expectResult: expectationDiffs{UseSurge: 1, UseSurgeOldRevision: 1, DeleteReadyLimit: 1, UpdateNum: 1, UpdateMaxUnavailable: 2},
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
			expectResult: expectationDiffs{UseSurge: 1, ScaleUpNum: 1, UpdateNum: 1, UpdateMaxUnavailable: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 0},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 1},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, UseSurge: 1, UpdateNum: 1, UpdateMaxUnavailable: 3, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 3},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, UseSurge: 1, UpdateNum: 1, UpdateMaxUnavailable: 2, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 2},
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
			expectResult: expectationDiffs{ScaleUpNum: 1, UseSurge: 1, UpdateNum: 1, UpdateMaxUnavailable: 1, ScaleUpLimit: 1},
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
			expectResult: expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 1, DeleteReadyLimit: 1},
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
			expectResult:       expectationDiffs{UpdateNum: 2, UpdateMaxUnavailable: 1},
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
			expectResult:       expectationDiffs{ScaleUpNum: 1, UpdateNum: 1, UpdateMaxUnavailable: 1, ScaleUpLimit: 1},
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
			expectResult:       expectationDiffs{ScaleDownNum: 1, ScaleDownNumOldRevision: 2, DeleteReadyLimit: 2, UpdateNum: 1, UpdateMaxUnavailable: 2},
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
			expectResult: expectationDiffs{ScaleUpNum: 5, ScaleUpLimit: 2},
		},
		{
			name: "increase replicas 3 to 6 with scale maxUnavailable = 2, not ready pod = 1",
			set:  setScaleStrategy(createTestCloneSet(6, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{ScaleUpNum: 3, ScaleUpLimit: 1},
		},
		{
			name: "increase replicas 3 to 6 with scale maxUnavailable = 2, not ready pod = 2",
			set:  setScaleStrategy(createTestCloneSet(6, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)), intstr.FromInt(2)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{ScaleUpNum: 3, ScaleUpLimit: 0},
		},
		{
			name: "[scalingExcludePreparingDelete=false] specific delete a pod with lifecycle hook (step 1/4)",
			set:  createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
			},
			expectResult: expectationDiffs{DeleteReadyLimit: 1},
		},
		{
			name: "[scalingExcludePreparingDelete=false] specific delete a pod with lifecycle hook (step 2/4)",
			set:  createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
			},
			expectResult: expectationDiffs{},
		},
		{
			name: "[scalingExcludePreparingDelete=false] specific delete a pod with lifecycle hook (step 3/4)",
			set:  createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
		},
		{
			name: "[scalingExcludePreparingDelete=false] specific delete a pod with lifecycle hook (step 4/4)",
			set:  createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook (step 1/4)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
			},
			expectResult: expectationDiffs{DeleteReadyLimit: 1},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook (step 2/4)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
			},
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook (step 3/4)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook (step 4/4)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook and then cancel (step 1/5)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
			},
			expectResult: expectationDiffs{DeleteReadyLimit: 1},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook and then cancel (step 2/5)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
			},
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook and then cancel (step 3/5)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook and then cancel (step 4/5)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false), // it has been changed to normal by managePreparingDelete
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{ScaleDownNum: 1, DeleteReadyLimit: 1},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific delete a pod with lifecycle hook and then cancel (step 5/5)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific scale down with lifecycle hook, then scale up pods (step 1/6)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific scale down with lifecycle hook, then scale up pods (step 2/6)",
			set:       createTestCloneSet(2, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, true),
			},
			expectResult: expectationDiffs{DeleteReadyLimit: 2},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific scale down with lifecycle hook, then scale up pods (step 3/6)",
			set:       createTestCloneSet(2, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific scale down with lifecycle hook, then scale up pods (step 4/6)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
			},
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific scale down with lifecycle hook, then scale up pods (step 5/6)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStatePreparingDelete, true, true),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingExcludePreparingDelete=true] specific scale down with lifecycle hook, then scale up pods (step 6/6)",
			set:       createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{},
		},
		{
			name:      "[scalingWithPreparingUpdate=true] scaling up when a preparing pod is not updated, and expected-updated is 1",
			set:       createTestCloneSet(4, intstr.FromString("90%"), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStatePreparingUpdate, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			isPodUpdate:  revision.IsPodUpdate,
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1, ScaleUpNumOldRevision: 1},
		},
		{
			name:      "[scalingWithPreparingUpdate=true] scaling up when a preparing pod is not updated, and expected-updated is 2",
			set:       createTestCloneSet(4, intstr.FromInt(2), intstr.FromInt(1), intstr.FromInt(0)),
			setLabels: map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"},
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStatePreparingUpdate, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			isPodUpdate:  revision.IsPodUpdate,
			expectResult: expectationDiffs{ScaleUpNum: 1, ScaleUpLimit: 1},
		},
		{
			name: "[UpdateStrategyPaused=true] scale up pods with maxSurge=3,maxUnavailable=0",
			set:  setUpdateStrategyPaused(createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(3)), true),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{ScaleUpNum: 2, ScaleUpLimit: 2, UpdateNum: 3, UpdateMaxUnavailable: -2},
		},
		{
			name: "[UpdateStrategyPaused=true] scale down pods with maxSurge=3,maxUnavailable=0",
			set:  setUpdateStrategyPaused(createTestCloneSet(3, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(3)), true),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
			},
			expectResult: expectationDiffs{ScaleDownNum: 2, ScaleDownNumOldRevision: 5, DeleteReadyLimit: 2, UpdateNum: 3, UpdateMaxUnavailable: 2},
		},
		{
			name: "[UpdateStrategyPaused=true] create 0 newRevision pods with maxSurge=3,maxUnavailable=0",
			set:  setUpdateStrategyPaused(createTestCloneSet(5, intstr.FromInt(0), intstr.FromInt(0), intstr.FromInt(3)), true),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{ScaleDownNum: 3, ScaleDownNumOldRevision: 5, UpdateNum: 2, UpdateMaxUnavailable: 3},
		},
		{
			name: "[UpdateStrategyPaused=true] create 0 newRevision pods with maxSurge=3,maxUnavailable=0",
			set:  setUpdateStrategyPaused(createTestCloneSet(5, intstr.FromInt(2), intstr.FromInt(0), intstr.FromInt(3)), true),
			pods: []*v1.Pod{
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(oldRevision, appspub.LifecycleStateNormal, true, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
				createTestPod(newRevision, appspub.LifecycleStateNormal, false, false),
			},
			expectResult: expectationDiffs{ScaleDownNum: 3, ScaleDownNumOldRevision: 3},
		},
	}

	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PreparingUpdateAsUpdate, true)()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.disableFeatureGate {
				_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.CloneSetPartitionRollback))
			} else {
				_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.CloneSetPartitionRollback))
			}
			current := oldRevision
			if tc.revisionConsistent {
				current = newRevision
			}
			for key, value := range tc.setLabels {
				if tc.set.Labels == nil {
					tc.set.Labels = map[string]string{}
				}
				tc.set.Labels[key] = value
			}
			res := calculateDiffsWithExpectation(tc.set, tc.pods, current, newRevision, tc.isPodUpdate)
			if !reflect.DeepEqual(res, tc.expectResult) {
				t.Errorf("got %#v, expect %#v", res, tc.expectResult)
			}
		})
	}
}

func TestMain(m *testing.M) {
	klog.InitFlags(nil)
	_ = flag.Set("v", "5")
	flag.Parse()
	defer klog.Flush()
	m.Run()
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

func setUpdateStrategyPaused(cs *appsv1alpha1.CloneSet, paused bool) *appsv1alpha1.CloneSet {
	cs.Spec.UpdateStrategy = appsv1alpha1.CloneSetUpdateStrategy{
		Partition:      cs.Spec.UpdateStrategy.Partition,
		MaxSurge:       cs.Spec.UpdateStrategy.MaxSurge,
		MaxUnavailable: cs.Spec.UpdateStrategy.MaxUnavailable,
		Paused:         paused,
	}
	return cs
}
