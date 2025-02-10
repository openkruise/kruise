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
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	testingclock "k8s.io/utils/clock/testing"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/apis"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
)

type manageCase struct {
	name           string
	cs             *appsv1alpha1.CloneSet
	updateRevision *apps.ControllerRevision
	revisions      []*apps.ControllerRevision
	pods           []*v1.Pod
	pvcs           []*v1.PersistentVolumeClaim
	expectedPods   []*v1.Pod
	expectedPVCs   []*v1.PersistentVolumeClaim
}

func (mc *manageCase) initial() []client.Object {
	var initialObjs []client.Object
	mc.cs.Name = "clone-test"
	initialObjs = append(initialObjs, mc.cs)

	for i := range mc.pods {
		initialObjs = append(initialObjs, mc.pods[i])
	}

	for i := range mc.pvcs {
		initialObjs = append(initialObjs, mc.pvcs[i])
	}

	return initialObjs
}

func getInt32Pointer(i int32) *int32 {
	return &i
}

func TestUpdate(t *testing.T) {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))
	now := metav1.NewTime(time.Unix(time.Now().Add(-time.Hour).Unix(), 0))
	cases := []manageCase{
		{
			name:           "do nothing",
			cs:             &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{Replicas: getInt32Pointer(1)}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "rev_new"}},
					Spec:       v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "rev_new", apps.DefaultDeploymentUniqueLabelKey: "rev_new"}},
					Spec:       v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name:           "normal update condition",
			cs:             &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{Replicas: getInt32Pointer(1)}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "rev_new"}},
					Spec:       v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "rev_new", apps.DefaultDeploymentUniqueLabelKey: "rev_new"}, ResourceVersion: "1"},
					Spec:       v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, LastTransitionTime: now, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "recreate update 1",
			cs: &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{
				Replicas:       getInt32Pointer(1),
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.RecreateCloneSetUpdateStrategyType},
			}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.CloneSetInstanceID:      "id-0",
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", ResourceVersion: "1", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.CloneSetInstanceID:      "id-0",
						appsv1alpha1.SpecifiedDeleteKey:      "true",
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPVCs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
		},
		{
			name: "recreate update 2",
			cs: &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{
				Replicas:       getInt32Pointer(1),
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType},
			}},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2","env":["name":"k", "value":"v"]}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.CloneSetInstanceID:      "id-0",
					}},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", ResourceVersion: "1", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.CloneSetInstanceID:      "id-0",
						appsv1alpha1.SpecifiedDeleteKey:      "true",
					}},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPVCs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
		},
		{
			name: "inplace update",
			cs: &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{
				Replicas:       getInt32Pointer(1),
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType},
			}},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.CloneSetInstanceID:      "id-0",
					}},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appsv1alpha1.CloneSetInstanceID:      "id-0",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateUpdating),
						},
						Annotations: map[string]string{appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
							Revision:               "rev_new",
							UpdateTimestamp:        now,
							UpdateImages:           true,
							LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "image-id-xyz"}},
							ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: now, Containers: []string{"c1"}}},
						})},
						ResourceVersion: "2",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo2"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPVCs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
		},
		{
			name: "inplace update with grace period",
			cs: &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{
				Replicas:       getInt32Pointer(1),
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType, InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 3630}},
			}},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.CloneSetInstanceID:      "id-0",
					}},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appsv1alpha1.CloneSetInstanceID:      "id-0",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateUpdating),
						},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:        "rev_new",
								UpdateTimestamp: now,
								UpdateImages:    true,
							}),
							appspub.InPlaceUpdateGraceKey: `{"revision":"rev_new","containerImages":{"c1":"foo2"},"graceSeconds":3630}`,
						},
						ResourceVersion: "2",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPVCs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
		},
		{
			name: "inplace update during grace period",
			cs: &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{
				Replicas:       getInt32Pointer(1),
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType, InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 3630}},
			}},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "rev_new", appsv1alpha1.CloneSetInstanceID: "id-0"},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:        "rev_new",
								UpdateTimestamp: metav1.NewTime(now.Add(-time.Second * 10)),
							}),
							appspub.InPlaceUpdateGraceKey: `{"revision":"rev_new","containerImages":{"c1":"foo2"},"graceSeconds":3630}`,
						},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appsv1alpha1.CloneSetInstanceID:      "id-0",
						},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:        "rev_new",
								UpdateTimestamp: metav1.NewTime(now.Add(-time.Second * 10)),
							}),
							appspub.InPlaceUpdateGraceKey: `{"revision":"rev_new","containerImages":{"c1":"foo2"},"graceSeconds":3630}`,
						},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPVCs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
		},
		{
			name: "inplace update continuously after grace period",
			cs: &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{
				Replicas:       getInt32Pointer(1),
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType, InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 3630}},
			}},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "rev_new", appsv1alpha1.CloneSetInstanceID: "id-0"},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:        "rev_new",
								UpdateTimestamp: metav1.NewTime(now.Add(-time.Minute)),
							}),
							appspub.InPlaceUpdateGraceKey: `{"revision":"rev_new","containerImages":{"c1":"foo2"},"graceSeconds":3630}`,
						},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			pvcs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appsv1alpha1.CloneSetInstanceID:      "id-0",
						},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:               "rev_new",
								UpdateTimestamp:        metav1.NewTime(now.Add(-time.Minute)),
								LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "image-id-xyz"}},
								ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: now, Containers: []string{"c1"}}},
							}),
						},
						ResourceVersion: "1",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo2"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPVCs: []*v1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-0", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "id-0"}}},
			},
		},
		{
			name:           "create: preparingNormal->Normal without hook",
			cs:             &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{Replicas: getInt32Pointer(1)}},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_new",
						apps.DefaultDeploymentUniqueLabelKey: "rev_new",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_new",
						apps.DefaultDeploymentUniqueLabelKey: "rev_new",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "create: preparingNormal->preparingNormal, preNormal does not hook",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas:  getInt32Pointer(1),
					Lifecycle: &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}}},
				},
			},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_new",
						apps.DefaultDeploymentUniqueLabelKey: "rev_new",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_new",
						apps.DefaultDeploymentUniqueLabelKey: "rev_new",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "create: preparingNormal->Normal, preNormal does hook",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas:  getInt32Pointer(1),
					Lifecycle: &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{LabelsHandler: map[string]string{"preNormalHooked": "true"}}},
				},
			},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						"preNormalHooked":                    "true",
						apps.ControllerRevisionHashLabelKey:  "rev_new",
						apps.DefaultDeploymentUniqueLabelKey: "rev_new",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						"preNormalHooked":                    "true",
						apps.ControllerRevisionHashLabelKey:  "rev_new",
						apps.DefaultDeploymentUniqueLabelKey: "rev_new",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "recreate update: preparingNormal, preNormal does not hook, specific delete",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas:  getInt32Pointer(1),
					Lifecycle: &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{"preNormalHooked"}}},
				},
			},
			updateRevision: &apps.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev_new"}},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey:  "rev_old",
						apps.DefaultDeploymentUniqueLabelKey: "rev_old",
						appsv1alpha1.SpecifiedDeleteKey:      "true",
						appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
					}},
					Spec: v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
					Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionTrue},
						{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue},
					}},
				},
			},
		},
		{
			name: "in-place update: preparingNormal->Updating, preNormal & InPlaceUpdate does not hook",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas:       getInt32Pointer(1),
					Lifecycle:      &appspub.Lifecycle{PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{"slb.com/online"}}},
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey:  "rev_new",
							apps.DefaultDeploymentUniqueLabelKey: "rev_new",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateUpdating),
						},
						Annotations: map[string]string{
							appspub.InPlaceUpdateStateKey: util.DumpJSON(appspub.InPlaceUpdateState{
								Revision:               "rev_new",
								UpdateImages:           true,
								UpdateTimestamp:        metav1.NewTime(now.Time),
								LastContainerStatuses:  map[string]appspub.InPlaceUpdateContainerStatus{"c1": {ImageID: "image-id-xyz"}},
								ContainerBatchesRecord: []appspub.InPlaceUpdateContainerBatch{{Timestamp: now, Containers: []string{"c1"}}},
							}),
						},
						ResourceVersion: "1",
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo2"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse, Reason: "StartInPlaceUpdate", LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
		{
			name: "in-place update: preparingNormal->Normal, preNormal & InPlaceUpdate does hook",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: getInt32Pointer(1),
					Lifecycle: &appspub.Lifecycle{
						PreNormal:     &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}},
						InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}},
					},
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingNormal),
						},
						Finalizers: []string{"slb/online"},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						},
						Finalizers: []string{"slb/online"},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
		{
			name: "in-place update: Normal->preparingUpdate, preNormal & InPlaceUpdate does hook",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: getInt32Pointer(1),
					Lifecycle: &appspub.Lifecycle{
						PreNormal:     &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}},
						InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{"slb/online"}},
					},
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType},
				},
			},
			updateRevision: &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{Name: "rev_new"},
				Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo2"}]}}}}`)},
			},
			revisions: []*apps.ControllerRevision{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "rev_old"},
					Data:       runtime.RawExtension{Raw: []byte(`{"spec":{"template":{"$patch":"replace","spec":{"containers":[{"name":"c1","image":"foo1"}]}}}}`)},
				},
			},
			pods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStateNormal),
						},
						Finalizers: []string{"slb/online"},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
			expectedPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-0",
						Labels: map[string]string{
							apps.DefaultDeploymentUniqueLabelKey: "rev_old",
							apps.ControllerRevisionHashLabelKey:  "rev_old",
							appspub.LifecycleStateKey:            string(appspub.LifecycleStatePreparingUpdate),
						},
						Finalizers: []string{"slb/online"},
					},
					Spec: v1.PodSpec{
						ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
						Containers:     []v1.Container{{Name: "c1", Image: "foo1"}},
					},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						Conditions: []v1.PodCondition{
							{Type: v1.PodReady, Status: v1.ConditionTrue},
							{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: now},
						},
						ContainerStatuses: []v1.ContainerStatus{{Name: "c1", ImageID: "image-id-xyz"}},
					},
				},
			},
		},
	}

	inplaceupdate.Clock = testingclock.NewFakeClock(now.Time)
	for _, mc := range cases {
		t.Run(mc.name, func(t *testing.T) {
			initialObjs := mc.initial()
			fakeClient := fake.NewClientBuilder().WithObjects(initialObjs...).Build()
			ctrl := &realControl{
				fakeClient,
				lifecycle.New(fakeClient),
				inplaceupdate.New(fakeClient, clonesetutils.RevisionAdapterImpl),
				record.NewFakeRecorder(10),
				&controllerfinder.ControllerFinder{Client: fakeClient},
			}
			currentRevision := mc.updateRevision
			if len(mc.revisions) > 0 {
				currentRevision = mc.revisions[0]
			}
			if err := ctrl.Update(mc.cs, currentRevision, mc.updateRevision, mc.revisions, mc.pods, mc.pvcs); err != nil {
				t.Fatalf("Failed to test %s, manage error: %v", mc.name, err)
			}
			podList := v1.PodList{}
			if err := ctrl.Client.List(context.TODO(), &podList, &client.ListOptions{}); err != nil {
				t.Fatalf("Failed to test %s, get pods error: %v", mc.name, err)
			}
			if len(podList.Items) != len(mc.expectedPods) {
				t.Fatalf("Failed to test %s, unexpected pods length, expected %v, got %v", mc.name, util.DumpJSON(mc.expectedPods), util.DumpJSON(podList.Items))
			}
			for _, p := range mc.expectedPods {
				p.APIVersion = "v1"
				p.Kind = "Pod"

				gotPod := &v1.Pod{}
				if err := ctrl.Client.Get(context.TODO(), types.NamespacedName{Namespace: p.Namespace, Name: p.Name}, gotPod); err != nil {
					t.Fatalf("Failed to test %s, get pod %s error: %v", mc.name, p.Name, err)
				}
				gotPod.APIVersion = "v1"
				gotPod.Kind = "Pod"

				if v, ok := gotPod.Annotations[appspub.LifecycleTimestampKey]; ok {
					if p.Annotations == nil {
						p.Annotations = map[string]string{}
					}
					p.Annotations[appspub.LifecycleTimestampKey] = v
				}
				p.ResourceVersion = gotPod.ResourceVersion

				if !reflect.DeepEqual(gotPod, p) {
					t.Fatalf("Failed to test %s, unexpected pod %s, expected \n%v\n got \n%v", mc.name, p.Name, util.DumpJSON(p), util.DumpJSON(gotPod))
				}
			}
		})
	}
}

func TestSortUpdateIndexes(t *testing.T) {
	cases := []struct {
		strategy          appsv1alpha1.CloneSetUpdateStrategy
		pods              []*v1.Pod
		waitUpdateIndexes []int
		expectedIndexes   []int
	}{
		{
			strategy: appsv1alpha1.CloneSetUpdateStrategy{},
			pods: []*v1.Pod{
				{Status: v1.PodStatus{Phase: v1.PodPending, Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}}},
				{Status: v1.PodStatus{Phase: v1.PodPending}},
				{Status: v1.PodStatus{Phase: v1.PodPending}},
				{Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}}},
				{Status: v1.PodStatus{Phase: v1.PodRunning}},
			},
			waitUpdateIndexes: []int{0, 1, 3, 4},
			expectedIndexes:   []int{1, 0, 4, 3},
		},
	}

	coreControl := clonesetcore.New(&appsv1alpha1.CloneSet{})
	for i, tc := range cases {
		got := SortUpdateIndexes(coreControl, tc.strategy, tc.pods, tc.waitUpdateIndexes)
		if !reflect.DeepEqual(got, tc.expectedIndexes) {
			t.Fatalf("case #%d failed, expected %v, got %v", i, tc.expectedIndexes, got)
		}
	}
}

func TestCalculateUpdateCount(t *testing.T) {
	// Enable the CloneSetPartitionRollback feature-gate
	_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.CloneSetPartitionRollback))

	readyPod := func() *v1.Pod {
		return &v1.Pod{Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}}}
	}
	cases := []struct {
		strategy           appsv1alpha1.CloneSetUpdateStrategy
		totalReplicas      int
		oldRevisionIndexes []int
		pods               []*v1.Pod
		expectedResult     int
	}{
		{
			strategy:           appsv1alpha1.CloneSetUpdateStrategy{},
			totalReplicas:      3,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{readyPod(), readyPod(), readyPod()},
			expectedResult:     1,
		},
		{
			strategy:           appsv1alpha1.CloneSetUpdateStrategy{},
			totalReplicas:      3,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{readyPod(), {}, readyPod()},
			expectedResult:     0,
		},
		{
			strategy:           appsv1alpha1.CloneSetUpdateStrategy{},
			totalReplicas:      3,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{{}, readyPod(), readyPod()},
			expectedResult:     1,
		},
		{
			strategy:           appsv1alpha1.CloneSetUpdateStrategy{},
			totalReplicas:      10,
			oldRevisionIndexes: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
			pods:               []*v1.Pod{{}, readyPod(), readyPod(), readyPod(), readyPod(), readyPod(), readyPod(), readyPod(), {}, readyPod()},
			expectedResult:     1,
		},
		{
			strategy:           appsv1alpha1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstrutil.FromInt(2)), MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(3))},
			totalReplicas:      3,
			oldRevisionIndexes: []int{0, 1},
			pods:               []*v1.Pod{{}, readyPod(), readyPod()},
			expectedResult:     0,
		},
		{
			strategy:           appsv1alpha1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstrutil.FromInt(2)), MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromString("50%"))},
			totalReplicas:      8,
			oldRevisionIndexes: []int{0, 1, 2, 3, 4, 5, 6},
			pods:               []*v1.Pod{{}, readyPod(), {}, readyPod(), readyPod(), readyPod(), readyPod(), {}},
			expectedResult:     3,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2, should only update 2 pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2, 3, 4},
			pods:               []*v1.Pod{{}, {}, {}, {}, {}},
			expectedResult:     2,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2, 2 updating, should not update pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{{}, {}, {}, {}, {}},
			expectedResult:     0,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2, 1 updated and 1 updating, should only update 1 pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{{}, {}, {}, readyPod(), {}},
			expectedResult:     1,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2， maxSurge = 1, 1 creating, should only update 2 pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
				MaxSurge:       intstrutil.ValueOrDefault(nil, intstrutil.FromInt(1)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2, 3, 4},
			pods:               []*v1.Pod{{}, {}, {}, {}, {}, {}},
			expectedResult:     2,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2， maxSurge = 1, 1 updated and 1 updating, should only update 2 pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
				MaxSurge:       intstrutil.ValueOrDefault(nil, intstrutil.FromInt(1)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2, 3},
			pods:               []*v1.Pod{{}, {}, {}, {}, readyPod(), {}},
			expectedResult:     2,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2， maxSurge = 1, 1 updated and 2 updating, should only update 1 pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
				MaxSurge:       intstrutil.ValueOrDefault(nil, intstrutil.FromInt(1)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{{}, {}, {}, {}, readyPod(), {}},
			expectedResult:     1,
		},
		{
			// old revision all unavailable, partition = 0, maxUnavailable = 2， maxSurge = 1, 3 updating, should not update pods
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(0)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
				MaxSurge:       intstrutil.ValueOrDefault(nil, intstrutil.FromInt(1)),
			},
			totalReplicas:      5,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{{}, {}, {}, {}, {}, {}},
			expectedResult:     0,
		},
		{
			// rollback with maxUnavailable and pods in new revision are unavailable
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				Partition:      util.GetIntOrStrPointer(intstrutil.FromInt(7)),
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
			},
			totalReplicas:      8,
			oldRevisionIndexes: []int{0, 1, 2},
			pods:               []*v1.Pod{readyPod(), readyPod(), readyPod(), {}, {}, {}, {}, {}},
			expectedResult:     2,
		},
		{
			// maxUnavailable = 0 and maxSurge = 2, usedSurge = 1
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(0)),
				MaxSurge:       intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
			},
			totalReplicas:      4,
			oldRevisionIndexes: []int{0, 1},
			pods:               []*v1.Pod{readyPod(), readyPod(), readyPod(), readyPod(), readyPod()},
			expectedResult:     1,
		},
		{
			// maxUnavailable = 0 and maxSurge = 2, usedSurge = 2
			strategy: appsv1alpha1.CloneSetUpdateStrategy{
				MaxUnavailable: intstrutil.ValueOrDefault(nil, intstrutil.FromInt(0)),
				MaxSurge:       intstrutil.ValueOrDefault(nil, intstrutil.FromInt(2)),
			},
			totalReplicas:      4,
			oldRevisionIndexes: []int{0, 1, 2, 3},
			pods:               []*v1.Pod{readyPod(), readyPod(), readyPod(), readyPod(), readyPod(), readyPod()},
			expectedResult:     2,
		},
	}

	coreControl := clonesetcore.New(&appsv1alpha1.CloneSet{})
	for i, tc := range cases {
		currentRevision := "current"
		updateRevision := "updated"
		indexes := sets.NewInt(tc.oldRevisionIndexes...)
		var newRevisionIndexes []int
		for i, pod := range tc.pods {
			if !indexes.Has(i) {
				newRevisionIndexes = append(newRevisionIndexes, i)
				pod.Labels = map[string]string{apps.ControllerRevisionHashLabelKey: updateRevision}
			} else {
				pod.Labels = map[string]string{apps.ControllerRevisionHashLabelKey: currentRevision}
			}
		}

		replicas := int32(tc.totalReplicas)
		cs := &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{Replicas: &replicas, UpdateStrategy: tc.strategy}}
		diffRes := calculateDiffsWithExpectation(cs, tc.pods, currentRevision, updateRevision, nil)

		var waitUpdateIndexes []int
		var targetRevision string
		if diffRes.updateNum > 0 {
			waitUpdateIndexes = tc.oldRevisionIndexes
			targetRevision = updateRevision
		} else if diffRes.updateNum < 0 {
			waitUpdateIndexes = newRevisionIndexes
			targetRevision = currentRevision
		}

		res := limitUpdateIndexes(coreControl, 0, diffRes, waitUpdateIndexes, tc.pods, targetRevision)
		if len(res) != tc.expectedResult {
			t.Fatalf("case #%d failed, expected %d, got %d", i, tc.expectedResult, res)
		}
	}
}
