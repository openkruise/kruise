package podprobemarker

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var (
	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Labels:      map[string]string{"app": "nginx", "pub-controller": "true"},
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "nginx",
					UID:        types.UID("606132e0-85ef-460a-8cf5-cd8f915a8cc3"),
					Controller: utilpointer.BoolPtr(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:v1",
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
					Image:   "nginx:v1",
					ImageID: "nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
					Ready:   true,
				},
			},
		},
	}
)

func TestPodUpdateEventHandler(t *testing.T) {
	newPod := podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())

	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	updateEvent := event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: podDemo,
		ObjectNew: newPod,
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := enqueueRequestForPod{reader: fakeClient}

	// update with pod no PodInitialized, then return
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// update with pod, pod is not active, then return
	newPod = podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	newPod.Status.Phase = corev1.PodSucceeded

	// update with pod status changed and reconcile
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// parse pod error
	updateEvent = event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: nil,
		ObjectNew: nil,
	}
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// parse old pod error
	updateEvent = event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: nil,
		ObjectNew: newPod,
	}
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

}

func TestPodUpdateEventHandler_v2(t *testing.T) {
	cases := []struct {
		name       string
		ppmList    *appsalphav1.PodProbeMarkerList
		getPod     func() (*corev1.Pod, *corev1.Pod)
		getNode    func() *corev1.Node
		expectQLen int
	}{
		{
			name: "podUpdateEvent, exist podProbeMarker",
			getPod: func() (*corev1.Pod, *corev1.Pod) {
				newPod := podDemo.DeepCopy()
				newPod.Spec.NodeName = "test-node"
				newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
				util.SetPodCondition(newPod, corev1.PodCondition{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionTrue,
				})
				oldPod := podDemo.DeepCopy()
				oldPod.Spec.NodeName = "test-node"
				util.SetPodCondition(oldPod, corev1.PodCondition{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionFalse,
				})
				return oldPod, newPod
			},
			getNode: func() *corev1.Node {
				return &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
				}
			},
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v1",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v2",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx-v2",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v3",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx-v3",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectQLen: 1,
		},
		{
			name: "podUpdateEvent, no exist podProbeMarker",
			getPod: func() (*corev1.Pod, *corev1.Pod) {
				newPod := podDemo.DeepCopy()
				newPod.Spec.NodeName = "test-node"
				newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
				util.SetPodCondition(newPod, corev1.PodCondition{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionTrue,
				})
				oldPod := podDemo.DeepCopy()
				oldPod.Spec.NodeName = "test-node"
				util.SetPodCondition(oldPod, corev1.PodCondition{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionFalse,
				})
				return oldPod, newPod
			},
			getNode: func() *corev1.Node {
				return &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
				}
			},
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v1",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx-v1",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v2",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx-v2",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v3",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx-v3",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectQLen: 0,
		},
		{
			name: "podUpdateEvent, serverless pods",
			getPod: func() (*corev1.Pod, *corev1.Pod) {
				newPod := podDemo.DeepCopy()
				newPod.Spec.NodeName = "test-node"
				newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
				util.SetPodCondition(newPod, corev1.PodCondition{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionTrue,
				})
				util.SetPodCondition(newPod, corev1.PodCondition{
					Type:   "game.kruise.io/idle",
					Status: corev1.ConditionTrue,
				})
				oldPod := podDemo.DeepCopy()
				oldPod.Spec.NodeName = "test-node"
				util.SetPodCondition(oldPod, corev1.PodCondition{
					Type:   corev1.PodInitialized,
					Status: corev1.ConditionFalse,
				})
				return oldPod, newPod
			},
			getNode: func() *corev1.Node {
				return &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"type": VirtualKubelet,
						},
					},
				}
			},
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v1",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v3",
							Namespace: "default",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "nginx",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "idle",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/idle.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/idle",
								},
							},
						},
					},
				},
			},
			expectQLen: 1,
		},
	}
	_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.EnablePodProbeMarkerOnServerless))
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			if cs.name != "podUpdateEvent, serverless pods" {
				return
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			handler := enqueueRequestForPod{reader: fakeClient}
			for _, ppm := range cs.ppmList.Items {
				_ = fakeClient.Create(context.TODO(), &ppm)
			}
			_ = fakeClient.Create(context.TODO(), cs.getNode())
			oldPod, newPod := cs.getPod()
			updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			updateEvent := event.TypedUpdateEvent[*corev1.Pod]{
				ObjectOld: oldPod,
				ObjectNew: newPod,
			}
			handler.Update(context.TODO(), updateEvent, updateQ)
			if updateQ.Len() != cs.expectQLen {
				t.Errorf("unexpected update event handle queue size, expected %d actual %d", cs.expectQLen, updateQ.Len())
			}
		})
	}
}
