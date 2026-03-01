/*
Copyright 2024 The Kruise Authors.

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
package statefulset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruisefake "github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
)

func newTestPVC(name string, compatible, ready bool, conditions []v1.PersistentVolumeClaimCondition) v1.PersistentVolumeClaim {
	return newTestPVCWithSC(name, nil, compatible, ready, conditions)
}

func newTestPVCWithSC(name string, scName *string, compatible, ready bool, conditions []v1.PersistentVolumeClaimCondition) v1.PersistentVolumeClaim {
	pvc := newPVC(name)
	pvc.Status.Capacity = map[v1.ResourceName]resource.Quantity{}
	if scName != nil {
		pvc.Spec.StorageClassName = scName
	}
	if compatible {
		pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")
	} else {
		pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("800Mi")
	}
	if ready {
		pvc.Status.Capacity[v1.ResourceStorage] = resource.MustParse("1Gi")
	} else {
		pvc.Status.Capacity[v1.ResourceStorage] = resource.MustParse("500Mi")
	}

	pvc.Status.Conditions = conditions
	return pvc
}

func TestIsOwnedPVCsReady(t *testing.T) {
	simpleSetFn := func(scs []*string) *appsv1beta1.StatefulSet {
		statefulSet := newStatefulSetWithGivenSC(5, len(scs), scs)
		statefulSet.Spec.VolumeClaimUpdateStrategy.Type = appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType
		return statefulSet
	}

	sc1 := newStorageClass("can_expand", true)
	sc2 := newStorageClass("cannot_expand", false)
	set := simpleSetFn([]*string{&sc1.Name, &sc1.Name})

	testCases := []struct {
		name        string
		pvcGetter   func() []*v1.PersistentVolumeClaim
		pod         *v1.Pod
		expectedErr bool
		expectedRes bool
	}{
		{
			name: "pvc1 is ready, pvc2 is not ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, false, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is not ready, pvc2 is ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, false, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "both pvc1 and pvc2 are ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
		{
			name: "pvc2 is missing",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-2-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pvcs := tc.pvcGetter()
			if len(pvcs) != 2 {
				t.Log("pvcLen != 2, skip")
				return
			}
			client := fake.NewSimpleClientset(&sc1, &sc2, pvcs[0], pvcs[1])
			kruiseClient := kruisefake.NewSimpleClientset(set)
			om, _, _, stop := setupController(client, kruiseClient)
			defer close(stop)

			spc := NewStatefulPodControlFromManager(om, &noopRecorder{})

			res, err := spc.IsOwnedPVCsReady(set, tc.pod)
			if tc.expectedRes != res {
				t.Errorf("expected res %v, got %v", tc.expectedRes, res)
			}
			if tc.expectedErr {
				t.Logf("expected err %v, got %v", tc.expectedErr, err)
				assert.NotNil(t, err)
			} else {
				t.Logf("expected err %v, got %v", tc.expectedErr, err)
				assert.Nil(t, err)
			}
		})
	}
}

func TestIsClaimsCompatible(t *testing.T) {
	simpleSetFn := func(scs []*string) *appsv1beta1.StatefulSet {
		statefulSet := newStatefulSetWithGivenSC(5, len(scs), scs)
		statefulSet.Spec.VolumeClaimUpdateStrategy.Type = appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType
		return statefulSet
	}

	sc1 := newStorageClass("can_expand", true)
	sc2 := newStorageClass("cannot_expand", false)
	set := simpleSetFn([]*string{&sc1.Name, &sc1.Name})
	set.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")
	set.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")
	testCases := []struct {
		name        string
		pvcGetter   func() []*v1.PersistentVolumeClaim
		pod         *v1.Pod
		expectedErr bool
		expectedRes bool
	}{
		{
			name: "pvc1 is compatible, pvc2 is not compatible",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", false, false, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is not compatible, pvc2 is compatible",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", false, false, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "both pvc1 and pvc2 are compatible",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
		{
			name: "pvc2 is missing",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-2-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pvcs := tc.pvcGetter()
			if len(pvcs) != 2 {
				t.Log("pvcLen != 2, skip")
				return
			}
			client := fake.NewSimpleClientset(&sc1, &sc2, pvcs[0], pvcs[1])
			kruiseClient := kruisefake.NewSimpleClientset(set)
			om, _, _, stop := setupController(client, kruiseClient)
			defer close(stop)

			spc := NewStatefulPodControlFromManager(om, &noopRecorder{})

			res, err := spc.IsClaimsCompatible(set, tc.pod)
			if tc.expectedRes != res {
				t.Errorf("expected res %v, got %v", tc.expectedRes, res)
			}
			if tc.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestTryPatchPVC(t *testing.T) {
	simpleSetFn := func(scs []*string) *appsv1beta1.StatefulSet {
		statefulSet := newStatefulSetWithGivenSC(5, len(scs), scs)
		statefulSet.Spec.VolumeClaimUpdateStrategy.Type = appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType
		return statefulSet
	}

	sc1 := newStorageClass("can_expand", true)
	sc2 := newStorageClass("cannot_expand", false)
	set := simpleSetFn([]*string{&sc1.Name, &sc2.Name})
	set.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")
	set.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")
	testCases := []struct {
		name        string
		updateSTSFn func(set *appsv1beta1.StatefulSet)
		pvcGetter   func() []*v1.PersistentVolumeClaim
		pod         *v1.Pod
		expectedErr bool
		expectedRes bool
	}{
		{
			name: "expand pvc can expand",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVCWithSC("datadir-0-foo-0", &sc1.Name, false, true, nil)
				pvc2 := newTestPVCWithSC("datadir-1-foo-0", &sc2.Name, true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
		},
		{
			name: "expand pvc can not expand",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVCWithSC("datadir-0-foo-0", &sc1.Name, true, true, nil)
				pvc2 := newTestPVCWithSC("datadir-1-foo-0", &sc2.Name, false, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: true,
		},
		{
			name: "expand both pvcs",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVCWithSC("datadir-0-foo-0", &sc1.Name, false, true, nil)
				pvc2 := newTestPVCWithSC("datadir-1-foo-0", &sc2.Name, false, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: true,
		},
		{
			name: "modify other field",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVCWithSC("datadir-0-foo-0", &sc1.Name, true, true, nil)
				scName := "ssss1"
				pvc2 := newTestPVCWithSC("datadir-1-foo-0", &scName, false, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: true,
		},
		{
			name: "sc not found",
			updateSTSFn: func(set *appsv1beta1.StatefulSet) {
				scName := "ssss1"
				set.Spec.VolumeClaimTemplates[1].Spec.StorageClassName = &scName
			},
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVCWithSC("datadir-0-foo-0", &sc1.Name, true, true, nil)
				scName := "ssss1"
				pvc2 := newTestPVCWithSC("datadir-1-foo-0", &scName, false, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: true,
		},
		{
			name: "pvc not found",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVCWithSC("datadir-0-foo-0", &sc1.Name, true, true, nil)
				pvc2 := newTestPVCWithSC("datadir-1-foo-1", &sc2.Name, false, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pvcs := tc.pvcGetter()
			if len(pvcs) != 2 {
				t.Log("pvcLen != 2, skip")
				return
			}
			if tc.updateSTSFn != nil {
				tc.updateSTSFn(set)
			}
			client := fake.NewSimpleClientset(&sc1, &sc2, pvcs[0], pvcs[1])
			kruiseClient := kruisefake.NewSimpleClientset(set)
			om, _, _, stop := setupController(client, kruiseClient)
			defer close(stop)

			spc := NewStatefulPodControlFromManager(om, &noopRecorder{})

			err := spc.TryPatchPVC(set, tc.pod)
			t.Log(err)
			if tc.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestIsOwnedPVCsCompleted(t *testing.T) {
	simpleSetFn := func(scs []*string) *appsv1beta1.StatefulSet {
		statefulSet := newStatefulSetWithGivenSC(5, len(scs), scs)
		statefulSet.Spec.VolumeClaimUpdateStrategy.Type = appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType
		return statefulSet
	}

	sc1 := newStorageClass("can_expand", true)
	sc2 := newStorageClass("cannot_expand", false)
	set := simpleSetFn([]*string{&sc1.Name, &sc1.Name})
	set.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")
	set.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("1Gi")

	testCases := []struct {
		name        string
		pvcGetter   func() []*v1.PersistentVolumeClaim
		pod         *v1.Pod
		expectedErr bool
		expectedRes bool
	}{
		{
			name: "pvc1 is compatible and ready, pvc2 is not ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, false, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is not ready, pvc2 is compatible and ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, false, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "both pvc1 and pvc2 are compatible and ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
		{
			// new pvc must be newest
			name: "pvc2 is missing",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, true, nil)
				pvc2 := newTestPVC("datadir-2-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
		{
			name: "both pvc1 and pvc2 are incompatible and ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", false, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", false, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is incompatible and ready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", false, true, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is compatible and unready",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, false, nil)
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is compatible and pending=true",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, false, []v1.PersistentVolumeClaimCondition{{
					Type:   v1.PersistentVolumeClaimFileSystemResizePending,
					Status: v1.ConditionTrue,
				}})
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: true,
		},
		{
			name: "pvc1 is compatible and pending=false",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, false, []v1.PersistentVolumeClaimCondition{{
					Type:   v1.PersistentVolumeClaimFileSystemResizePending,
					Status: v1.ConditionFalse,
				}})
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
		{
			name: "pvc1 is compatible and resizing=true",
			pvcGetter: func() []*v1.PersistentVolumeClaim {
				pvc1 := newTestPVC("datadir-0-foo-0", true, false, []v1.PersistentVolumeClaimCondition{{
					Type:   v1.PersistentVolumeClaimResizing,
					Status: v1.ConditionTrue,
				}})
				pvc2 := newTestPVC("datadir-1-foo-0", true, true, nil)
				return []*v1.PersistentVolumeClaim{&pvc1, &pvc2}
			},
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
			},
			expectedErr: false,
			expectedRes: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pvcs := tc.pvcGetter()
			if len(pvcs) != 2 {
				t.Log("pvcLen != 2, skip")
				return
			}
			client := fake.NewSimpleClientset(&sc1, &sc2, pvcs[0], pvcs[1])
			kruiseClient := kruisefake.NewSimpleClientset(set)
			om, _, _, stop := setupController(client, kruiseClient)
			defer close(stop)

			spc := NewStatefulPodControlFromManager(om, &noopRecorder{})

			res, err := spc.IsOwnedPVCsCompleted(set, tc.pod)
			if tc.expectedRes != res {
				t.Errorf("expected res %v, got %v", tc.expectedRes, res)
			}
			if err != nil {
				t.Log(err)
			}
			if tc.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
