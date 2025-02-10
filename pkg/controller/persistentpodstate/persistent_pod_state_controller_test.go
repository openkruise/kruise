/*
Copyright 2022 The Kruise Authors.

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

package persistentpodstate

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/fieldindex"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/apis/apps"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	scheme *runtime.Scheme

	kruiseStsDemo = appsv1beta1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: KruiseKindSts.GroupVersion().String(),
			Kind:       KruiseKindSts.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-test",
			Name:      "test-sts",
			UID:       "012d18d5-5eb9-449d-b670-3da8fec8852f",
		},
		Spec: appsv1beta1.StatefulSetSpec{
			Replicas: ptr.To[int32](10),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
	}

	podDemo = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-test",
			OwnerReferences: []metav1.OwnerReference{
				{
					Controller: ptr.To(true),
				},
			},
			Annotations: map[string]string{
				"test-annotations": "test-value",
			},
			Labels: map[string]string{
				"test-labels": "test-value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node/gpu",
										Operator: corev1.NodeSelectorOpExists,
									},
								},
							},
						},
					},
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
		},
	}

	nodeDemo = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				podStateZoneTopologyLabel: "cn-beijing",
				podStateNodeTopologyLabel: "kube-resource011162007216",
			},
		},
	}

	staticIPDemo = appsv1alpha1.PersistentPodState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "ns-test",
		},
		Spec: appsv1alpha1.PersistentPodStateSpec{
			TargetReference: appsv1alpha1.TargetReference{
				APIVersion: KruiseKindSts.GroupVersion().String(),
				Kind:       KruiseKindSts.Kind,
				Name:       "test-sts",
			},
			RequiredPersistentTopology: &appsv1alpha1.NodeTopologyTerm{
				NodeTopologyKeys: []string{podStateZoneTopologyLabel},
			},
			PreferredPersistentTopology: []appsv1alpha1.PreferredTopologyTerm{
				{
					Weight: 10,
					Preference: appsv1alpha1.NodeTopologyTerm{
						NodeTopologyKeys: []string{podStateNodeTopologyLabel},
					},
				},
			},
		},
		Status: appsv1alpha1.PersistentPodStateStatus{
			PodStates: map[string]appsv1alpha1.PodState{},
		},
	}

	podStateZoneTopologyLabel = "topology.kubernetes.io/zone"
	podStateNodeTopologyLabel = "kubernetes.io/hostname"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
}

func TestReconcilePersistentPodState(t *testing.T) {
	cases := []struct {
		name                     string
		getSts                   func() (*apps.StatefulSet, *appsv1beta1.StatefulSet)
		getPods                  func() []*corev1.Pod
		getNodes                 func() []*corev1.Node
		getPersistentPodState    func() *appsv1alpha1.PersistentPodState
		exceptPersistentPodState func() *appsv1alpha1.PersistentPodState
	}{
		{
			name: "kruise statefulset, 10 ready pod, and create staticIP",
			getSts: func() (*apps.StatefulSet, *appsv1beta1.StatefulSet) {
				return nil, kruiseStsDemo.DeepCopy()
			},
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				for i := 0; i < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					pod.OwnerReferences[0].UID = kruiseStsDemo.UID
					pods = append(pods, pod)
				}
				return pods
			},
			getNodes: func() []*corev1.Node {
				nodes := make([]*corev1.Node, 0)
				for i := 0; i < 10; i++ {
					node := nodeDemo.DeepCopy()
					node.Name = fmt.Sprintf("node-%d", i)
					node.Labels[podStateZoneTopologyLabel] = fmt.Sprintf("cn-beijing-%d", i)
					node.Labels[podStateNodeTopologyLabel] = fmt.Sprintf("kube-resource011162007216-%d", i)
					nodes = append(nodes, node)
				}
				return nodes
			},
			getPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				return staticIP
			},
			exceptPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("node-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
		},
		{
			name: "kruise statefulset, scale down replicas 10->8, 1 pod deleted, 1 pod running",
			getSts: func() (*apps.StatefulSet, *appsv1beta1.StatefulSet) {
				kruise := kruiseStsDemo.DeepCopy()
				kruise.Spec.Replicas = ptr.To[int32](8)
				return nil, kruise
			},
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				for i := 0; i < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					pod.OwnerReferences[0].UID = kruiseStsDemo.UID
					pod.Status = corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					}
					// 0, 3 not ready
					if i == 0 || i == 3 {
						pod.Status.Conditions[0].Status = corev1.ConditionFalse
					}
					// 9 is deleted, but 8 is running
					if i == 9 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					pods = append(pods, pod)
				}
				return pods
			},
			getNodes: func() []*corev1.Node {
				nodes := make([]*corev1.Node, 0)
				for i := 0; i < 10; i++ {
					node := nodeDemo.DeepCopy()
					node.Name = fmt.Sprintf("node-%d", i)
					node.Labels[podStateZoneTopologyLabel] = fmt.Sprintf("cn-beijing-%d", i)
					node.Labels[podStateNodeTopologyLabel] = fmt.Sprintf("kube-resource011162007216-%d", i)
					nodes = append(nodes, node)
				}
				return nodes
			},
			getPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				staticIP.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: kruiseStsDemo.APIVersion,
						Kind:       kruiseStsDemo.Kind,
						Name:       kruiseStsDemo.Name,
						UID:        kruiseStsDemo.UID,
					},
				}
				for i := 0; i < 10; i++ {
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("node-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
			exceptPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				staticIP.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: kruiseStsDemo.APIVersion,
						Kind:       kruiseStsDemo.Kind,
						Name:       kruiseStsDemo.Name,
						UID:        kruiseStsDemo.UID,
					},
				}
				for i := 0; i < 10; i++ {
					if i == 9 {
						continue
					}
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("node-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
		},
		{
			name: "kruise reserveOrigin statefulset, scale down replicas 10->8, 1 pod deleted, 1 pod running",
			getSts: func() (*apps.StatefulSet, *appsv1beta1.StatefulSet) {
				kruise := kruiseStsDemo.DeepCopy()
				kruise.Spec.Replicas = ptr.To[int32](8)
				kruise.Spec.ReserveOrdinals = []intstr.IntOrString{
					intstr.FromInt32(0),
					intstr.FromInt32(3),
					intstr.FromInt32(7),
				}
				return nil, kruise
			},
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				for i := 0; i < 13; i++ {
					//reserved
					if i == 0 || i == 3 || i == 7 {
						continue
					}
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					pod.OwnerReferences[0].UID = kruiseStsDemo.UID
					pod.Spec.NodeName = fmt.Sprintf("node-%d", i)
					pod.Status = corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					}
					// 12 is deleted, but 11 is running
					if i == 12 {
						pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						pod.Finalizers = []string{"finalizers.sigs.k8s.io/test"}
					}
					pods = append(pods, pod)
				}
				return pods
			},
			getNodes: func() []*corev1.Node {
				nodes := make([]*corev1.Node, 0)
				for i := 0; i < 13; i++ {
					//reserved
					if i == 0 || i == 3 || i == 7 {
						continue
					}
					node := nodeDemo.DeepCopy()
					node.Name = fmt.Sprintf("node-%d", i)
					node.Labels[podStateZoneTopologyLabel] = fmt.Sprintf("cn-beijing-%d", i)
					node.Labels[podStateNodeTopologyLabel] = fmt.Sprintf("kube-resource011162007216-%d", i)
					nodes = append(nodes, node)
				}
				return nodes
			},
			getPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				staticIP.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: kruiseStsDemo.APIVersion,
						Kind:       kruiseStsDemo.Kind,
						Name:       kruiseStsDemo.Name,
						UID:        kruiseStsDemo.UID,
					},
				}
				for i := 0; i < 13; i++ {
					//reserved
					if i == 0 || i == 3 || i == 7 {
						continue
					}
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("node-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
			exceptPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				staticIP.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: kruiseStsDemo.APIVersion,
						Kind:       kruiseStsDemo.Kind,
						Name:       kruiseStsDemo.Name,
						UID:        kruiseStsDemo.UID,
					},
				}
				for i := 0; i < 13; i++ {
					//reserved
					if i == 0 || i == 3 || i == 7 {
						continue
					}
					//deleted
					if i == 12 {
						continue
					}
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("node-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
		},
		{
			name: "kruise statefulset, 8 running pod, 2 from not ready to ready, and add staticIP in staticIP",
			getSts: func() (*apps.StatefulSet, *appsv1beta1.StatefulSet) {
				return nil, kruiseStsDemo.DeepCopy()
			},
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				for i := 0; i < 10; i++ {
					pod := podDemo.DeepCopy()
					pod.Name = fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					pod.OwnerReferences[0].UID = kruiseStsDemo.UID
					pod.Status = corev1.PodStatus{
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
					}
					pods = append(pods, pod)
				}
				return pods
			},
			getNodes: func() []*corev1.Node {
				nodes := make([]*corev1.Node, 0)
				for i := 0; i < 10; i++ {
					node := nodeDemo.DeepCopy()
					node.Name = fmt.Sprintf("node-%d", i)
					node.Labels[podStateZoneTopologyLabel] = fmt.Sprintf("cn-beijing-%d", i)
					node.Labels[podStateNodeTopologyLabel] = fmt.Sprintf("kube-resource011162007216-%d", i)
					nodes = append(nodes, node)
				}
				return nodes
			},
			getPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					if i == 5 || i == 6 {
						continue
					}
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("cn-beijing-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
			exceptPersistentPodState: func() *appsv1alpha1.PersistentPodState {
				staticIP := staticIPDemo.DeepCopy()
				for i := 0; i < 10; i++ {
					key := fmt.Sprintf("%s-%d", kruiseStsDemo.Name, i)
					staticIP.Status.PodStates[key] = appsv1alpha1.PodState{
						NodeName: fmt.Sprintf("node-%d", i),
						NodeTopologyLabels: map[string]string{
							podStateZoneTopologyLabel: fmt.Sprintf("cn-beijing-%d", i),
							podStateNodeTopologyLabel: fmt.Sprintf("kube-resource011162007216-%d", i),
						},
					}
				}
				return staticIP
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			nodes := cs.getNodes()
			pods := cs.getPods()
			for i, pod := range pods {
				pod.Spec.NodeName = nodes[i].Name
			}
			sts, kruiseSts := cs.getSts()
			staticIP := cs.getPersistentPodState()

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "ns-test",
					Name:      "test-sts",
				},
			}

			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if staticIP != nil {
				clientBuilder.WithObjects(staticIP)
			}
			if sts != nil {
				clientBuilder.WithObjects(sts)
			} else {
				clientBuilder.WithObjects(kruiseSts)
			}
			for _, node := range nodes {
				clientBuilder.WithObjects(node)
			}
			for _, pod := range pods {
				clientBuilder.WithObjects(pod)
			}
			clientBuilder.WithStatusSubresource(&appsv1alpha1.PersistentPodState{})
			fakeClient := clientBuilder.WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
				var owners []string
				for _, ref := range obj.GetOwnerReferences() {
					owners = append(owners, string(ref.UID))
				}
				return owners
			}).Build()
			reconciler := ReconcilePersistentPodState{
				Client: fakeClient,
				finder: &controllerfinder.ControllerFinder{Client: fakeClient},
			}
			if _, err := reconciler.Reconcile(context.TODO(), request); err != nil {
				t.Fatalf("reconcile failed, err: %v", err)
			}

			latestPersistentPodState, err := getLatestPersistentPodState(fakeClient, staticIPDemo.DeepCopy())
			if err != nil {
				t.Fatalf("get latest pod failed, err: %v", err)
			}
			if !reflect.DeepEqual(latestPersistentPodState.Status, cs.exceptPersistentPodState().Status) {
				t.Fatalf("staticIP deepequal failed")
			}
		})
	}
}

func getLatestPersistentPodState(client client.Client, staticIP *appsv1alpha1.PersistentPodState) (*appsv1alpha1.PersistentPodState, error) {
	newPersistentPodState := &appsv1alpha1.PersistentPodState{}
	Key := types.NamespacedName{
		Namespace: staticIP.Namespace,
		Name:      staticIP.Name,
	}
	err := client.Get(context.TODO(), Key, newPersistentPodState)
	return newPersistentPodState, err
}
