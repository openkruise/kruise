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

package sidecarset

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	utilpointer "k8s.io/utils/pointer"
)

type FactorySidecarSet func() *appsv1alpha1.SidecarSet
type FactoryPods func(int, int, int) []*corev1.Pod

func factoryPodsCommon(count, upgraded int, sidecarSet *appsv1alpha1.SidecarSet) []*corev1.Pod {
	control := sidecarcontrol.New(sidecarSet)
	pods := make([]*corev1.Pod, 0, count)
	for i := 0; i < count; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					sidecarcontrol.SidecarSetHashAnnotation:             `{"test-sidecarset":{"hash":"aaa","sidecarList":["test-sidecar"]}}`,
					sidecarcontrol.SidecarSetHashWithoutImageAnnotation: `{"test-sidecarset":{"hash":"without-aaa","sidecarList":["test-sidecar"]}}`,
				},
				Name: fmt.Sprintf("pod-%d", i),
				Labels: map[string]string{
					"app": "sidecar",
				},
				CreationTimestamp: metav1.Now(),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:1.15.1",
					},
					{
						Name:  "test-sidecar",
						Image: "test-image:v1",
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:    "nginx",
						Image:   "nginx:1.15.1",
						ImageID: "docker-pullable://nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
						Ready:   true,
					},
					{
						Name:    "test-sidecar",
						Image:   "test-image:v1",
						ImageID: testImageV1ImageID,
						Ready:   true,
					},
				},
			},
		}
		pods = append(pods, pod)
	}
	for i := 0; i < upgraded; i++ {
		pods[i].Spec.Containers[1].Image = "test-image:v2"
		sidecarcontrol.UpdatePodSidecarSetHash(pods[i], control.GetSidecarset())
		control.UpdatePodAnnotationsInUpgrade([]string{"test-sidecar"}, pods[i])
	}
	return pods
}

func factoryPods(count, upgraded, upgradedAndReady int) []*corev1.Pod {
	sidecarSet := factorySidecarSet()
	pods := factoryPodsCommon(count, upgraded, sidecarSet)
	for i := 0; i < upgradedAndReady; i++ {
		pods[i].Status.ContainerStatuses[1].Image = "test-image:v2"
		pods[i].Status.ContainerStatuses[1].ImageID = testImageV2ImageID
	}

	return pods
}

func factorySidecarSet() *appsv1alpha1.SidecarSet {
	return createFactorySidecarSet("bbb", "without-aaa")
}

func factorySidecarSetNotUpgradable() *appsv1alpha1.SidecarSet {
	return createFactorySidecarSet("bbb", "without-bbb")
}

func createFactorySidecarSet(sidecarsetHash string, sidecarsetHashWithoutImage string) *appsv1alpha1.SidecarSet {
	sidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation:             sidecarsetHash,
				sidecarcontrol.SidecarSetHashWithoutImageAnnotation: sidecarsetHashWithoutImage,
			},
			Name:   "test-sidecarset",
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "test-sidecar",
						Image: "test-image:v2",
					},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "sidecar"},
			},
			UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
				//Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			},
			RevisionHistoryLimit: utilpointer.Int32Ptr(10),
		},
	}

	return sidecarSet
}

func TestGetNextUpgradePods(t *testing.T) {
	testGetNextUpgradePods(t, factoryPods, factorySidecarSet)
}

func testGetNextUpgradePods(t *testing.T, factoryPods FactoryPods, factorySidecar FactorySidecarSet) {
	cases := []struct {
		name                     string
		getPods                  func() []*corev1.Pod
		getSidecarset            func() *appsv1alpha1.SidecarSet
		exceptNeedUpgradeCount   int
		exceptNotUpgradableCount int
	}{
		{
			name: "only maxUnavailable(int=10), and pods(count=100, upgraded=30, upgradedAndReady=26)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 30, 26)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   6,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "only maxUnavailable(string=10%), and pods(count=1000, upgraded=300, upgradedAndReady=260)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 300, 260)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "10%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   60,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "only maxUnavailable(string=5%), and pods(count=1000, upgraded=300, upgradedAndReady=250)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 300, 250)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "5%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   0,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "only maxUnavailable(int=100), and pods(count=100, upgraded=30, upgradedAndReady=27)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 30, 27)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   70,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "partition(int=180) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 180,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   20,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "partition(int=100) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   60,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "partition(string=18%) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "18%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   20,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "partition(string=10%) maxUnavailable(int=100), and pods(count=1000, upgraded=800, upgradedAndReady=760)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 800, 760)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "10%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   60,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "selector(app=test, count=30) maxUnavailable(int=100), and pods(count=1000, upgraded=0, upgradedAndReady=0)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(1000, 0, 0)
				for i := 0; i < 30; i++ {
					pods[i].Labels["app"] = "test"
				}
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}
				sidecarSet.Spec.UpdateStrategy.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   30,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "not upgradable sidecarset, maxUnavailable(int=100), and pods(count=100, upgraded=0, upgradedAndReady=0)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 0, 0)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecarSetNotUpgradable()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 100,
				}

				return sidecarSet
			},
			exceptNeedUpgradeCount:   0,
			exceptNotUpgradableCount: 100,
		},
		{
			name: "only maxUnavailable(5%), and pods(count=5, upgraded=0, upgradedAndReady=0)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(5, 0, 0)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "5%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   1,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "maxUnavailable(5) partition(99%), and pods(count=5, upgraded=0, upgradedAndReady=0)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(5, 0, 0)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 5,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "99%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   1,
			exceptNotUpgradableCount: 0,
		},
		{
			name: "maxUnavailable(5) partition(1%), and pods(count=5, upgraded=0, upgradedAndReady=0)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(5, 0, 0)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 5,
				}
				sidecarSet.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "1%",
				}
				return sidecarSet
			},
			exceptNeedUpgradeCount:   4,
			exceptNotUpgradableCount: 0,
		},
	}
	strategy := NewStrategy()
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := sidecarcontrol.New(cs.getSidecarset())
			pods := cs.getPods()
			upgradePods, notUpgradablePods := strategy.GetNextUpgradePods(control, pods)
			if cs.exceptNeedUpgradeCount != len(upgradePods) {
				t.Fatalf("except NeedUpgradeCount(%d), but get value(%d)", cs.exceptNeedUpgradeCount, len(upgradePods))
			}
			if cs.exceptNotUpgradableCount != len(notUpgradablePods) {
				t.Fatalf("except NotUpgradableCount(%d), but get value(%d)", cs.exceptNotUpgradableCount, len(notUpgradablePods))
			}
		})
	}
}

func TestParseUpdateScatterTerms(t *testing.T) {
	cases := []struct {
		name                  string
		getPods               func() []*corev1.Pod
		getScatterStrategy    func() appsv1alpha1.UpdateScatterStrategy
		exceptScatterStrategy func() appsv1alpha1.UpdateScatterStrategy
	}{
		{
			name: "only scatter terms",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 0, 0)
				return pods
			},
			getScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
				}
				return scatter
			},
			exceptScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
				}
				return scatter
			},
		},
		{
			name: "regular and scatter terms",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(100, 0, 0)
				pods[0].Labels["key-4"] = "value-4-0"
				pods[1].Labels["key-4"] = "value-4-1"
				pods[2].Labels["key-4"] = "value-4-2"
				pods[3].Labels["key-4"] = "value-4"
				pods[4].Labels["key-4"] = "value-4"
				pods[5].Labels["key-4"] = "value-4"
				return pods
			},
			getScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
					{
						Key:   "key-4",
						Value: "*",
					},
				}
				return scatter
			},
			exceptScatterStrategy: func() appsv1alpha1.UpdateScatterStrategy {
				scatter := appsv1alpha1.UpdateScatterStrategy{
					{
						Key:   "key-1",
						Value: "value-1",
					},
					{
						Key:   "key-2",
						Value: "value-2",
					},
					{
						Key:   "key-3",
						Value: "value-3",
					},
					{
						Key:   "key-4",
						Value: "value-4-0",
					},
					{
						Key:   "key-4",
						Value: "value-4-1",
					},
					{
						Key:   "key-4",
						Value: "value-4-2",
					},
					{
						Key:   "key-4",
						Value: "value-4",
					},
				}
				return scatter
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pods := cs.getPods()
			scatter := cs.getScatterStrategy()
			exceptScatter := cs.exceptScatterStrategy()
			newScatter := parseUpdateScatterTerms(scatter, pods)
			if !reflect.DeepEqual(newScatter, exceptScatter) {
				except, _ := json.Marshal(exceptScatter)
				new, _ := json.Marshal(newScatter)
				t.Fatalf("except scatter(%s), but get scatter(%s)", string(except), string(new))
			}
		})
	}
}

func Random(pods []*corev1.Pod) []*corev1.Pod {
	for i := len(pods) - 1; i > 0; i-- {
		num := rand.Intn(i + 1)
		pods[i], pods[num] = pods[num], pods[i]
	}
	return pods
}

func TestSortNextUpgradePods(t *testing.T) {
	testSortNextUpgradePods(t, factoryPods, factorySidecarSet)
}

func testSortNextUpgradePods(t *testing.T, factoryPods FactoryPods, factorySidecar FactorySidecarSet) {
	cases := []struct {
		name                  string
		getPods               func() []*corev1.Pod
		getSidecarset         func() *appsv1alpha1.SidecarSet
		exceptNextUpgradePods []string
	}{
		{
			name: "sort by pod.CreationTimestamp, maxUnavailable(int=10) and pods(count=20, upgraded=10, upgradedAndReady=5)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(20, 10, 5)
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				return sidecarSet
			},
			exceptNextUpgradePods: []string{"pod-19", "pod-18", "pod-17", "pod-16", "pod-15"},
		},
		{
			name: "not ready priority, maxUnavailable(int=10) and pods(count=20, upgraded=10, upgradedAndReady=5)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(20, 10, 5)
				podutil.GetPodReadyCondition(pods[10].Status).Status = corev1.ConditionFalse
				podutil.GetPodReadyCondition(pods[13].Status).Status = corev1.ConditionFalse
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				return sidecarSet
			},
			exceptNextUpgradePods: []string{"pod-13", "pod-10", "pod-19", "pod-18", "pod-17", "pod-16", "pod-15"},
		},
		{
			name: "with weight priority strategy, maxUnavailable(int=10) and pods(count=20, upgraded=10, upgradedAndReady=2)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(20, 10, 2)
				pods[15].Labels["test-key"] = "bar"
				pods[16].Labels["test-key"] = "foo"
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				sidecarSet.Spec.UpdateStrategy.PriorityStrategy = &appspub.UpdatePriorityStrategy{
					WeightPriority: []appspub.UpdatePriorityWeightTerm{
						{
							Weight: 50,
							MatchSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"test-key": "foo",
								},
							},
						},
						{
							Weight: 30,
							MatchSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"test-key": "bar",
								},
							},
						},
					},
				}
				return sidecarSet
			},
			exceptNextUpgradePods: []string{"pod-16", "pod-15"},
		},
		{
			name: "with order priority strategy, maxUnavailable(int=10) and pods(count=20, upgraded=10, upgradedAndReady=2)",
			getPods: func() []*corev1.Pod {
				pods := factoryPods(20, 10, 2)
				for i := 0; i < 20; i++ {
					pods[i].Labels["key1"] = "5"
				}
				pods[17].Labels["key1"] = "10"
				pods[18].Labels["key1"] = "20"
				return Random(pods)
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				sidecarSet := factorySidecar()
				sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 10,
				}
				sidecarSet.Spec.UpdateStrategy.PriorityStrategy = &appspub.UpdatePriorityStrategy{
					OrderPriority: []appspub.UpdatePriorityOrderTerm{
						{
							OrderedKey: "key1",
						},
					},
				}
				return sidecarSet
			},
			exceptNextUpgradePods: []string{"pod-18", "pod-17"},
		},
	}

	strategy := NewStrategy()
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := sidecarcontrol.New(cs.getSidecarset())
			pods := cs.getPods()
			injectedPods, _ := strategy.GetNextUpgradePods(control, pods)
			if len(cs.exceptNextUpgradePods) != len(injectedPods) {
				t.Fatalf("except NeedUpgradeCount(%d), but get value(%d)", len(cs.exceptNextUpgradePods), len(injectedPods))
			}

			for i, name := range cs.exceptNextUpgradePods {
				if injectedPods[i].Name != name {
					t.Fatalf("except NextUpgradePods[%d:%s], but get pods[%s]", i, name, injectedPods[i])
				}
			}
		})
	}
}
