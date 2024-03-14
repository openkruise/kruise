package podprobemarker

import (
	"context"
	"fmt"
	"reflect"
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
	"github.com/openkruise/kruise/pkg/util"
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
	updateEvent := event.UpdateEvent{
		ObjectOld: podDemo,
		ObjectNew: newPod,
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := enqueueRequestForPod{reader: fakeClient}

	// update with pod no PodInitialized, then return
	handler.Update(updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// update with pod, pod is not active, then return
	newPod = podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	newPod.Status.Phase = corev1.PodSucceeded

	// update with pod status changed and reconcile
	handler.Update(updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// parse pod error
	updateEvent = event.UpdateEvent{
		ObjectOld: nil,
		ObjectNew: nil,
	}
	handler.Update(updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// parse old pod error
	updateEvent = event.UpdateEvent{
		ObjectOld: nil,
		ObjectNew: newPod,
	}
	handler.Update(updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

}

func TestPodUpdateEventHandler_v2(t *testing.T) {
	cases := []struct {
		name       string
		ppmList    *appsalphav1.PodProbeMarkerList
		expectQLen int
	}{
		{
			name: "podUpdateEvent, exist podProbeMarker",
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
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			handler := enqueueRequestForPod{reader: fakeClient}
			for _, ppm := range cs.ppmList.Items {
				fakeClient.Create(context.TODO(), &ppm)
			}
			newPod := podDemo.DeepCopy()
			newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
			util.SetPodCondition(newPod, corev1.PodCondition{
				Type:   corev1.PodInitialized,
				Status: corev1.ConditionTrue,
			})
			util.SetPodCondition(podDemo, corev1.PodCondition{
				Type:   corev1.PodInitialized,
				Status: corev1.ConditionFalse,
			})

			updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			updateEvent := event.UpdateEvent{
				ObjectOld: podDemo,
				ObjectNew: newPod,
			}
			handler.Update(updateEvent, updateQ)
			if updateQ.Len() != cs.expectQLen {
				t.Errorf("unexpected update event handle queue size, expected %v actual %d", cs.expectQLen, updateQ.Len())
			}
		})
	}
}

func TestGetPodProbeMarkerForPod(t *testing.T) {

	cases := []struct {
		name      string
		ppmList   *appsalphav1.PodProbeMarkerList
		pod       *corev1.Pod
		expect    []*appsalphav1.PodProbeMarker
		expectErr error
	}{
		{
			name: "get pod probe marker list resources",
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v1",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v1",
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
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
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
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
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
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Labels: map[string]string{
						"app": "test-v2",
					},
				},
			},
			expect: []*appsalphav1.PodProbeMarker{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "game-server-probe-v2",
						Namespace:       "sp1",
						ResourceVersion: "1",
					},
					Spec: appsalphav1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test-v2",
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
						Name:            "game-server-probe-v3",
						Namespace:       "sp1",
						ResourceVersion: "1",
					},
					Spec: appsalphav1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test-v2",
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
			expectErr: nil,
		},
		{
			name: "no found pod probe marker list resources",
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "game-server-probe-v1",
							Namespace:       "sp1",
							ResourceVersion: "01",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v1",
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
							Name:            "game-server-probe-v2",
							Namespace:       "sp1",
							ResourceVersion: "01",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
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
							Name:            "game-server-probe-v3",
							Namespace:       "sp1",
							ResourceVersion: "01",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
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
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Labels: map[string]string{
						"app": "test",
					},
				},
			},
			expect:    nil,
			expectErr: nil,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, ppm := range cs.ppmList.Items {
				fakeClient.Create(context.TODO(), &ppm)
			}
			handler := enqueueRequestForPod{reader: fakeClient}
			get, err := handler.getPodProbeMarkerForPod(cs.pod)
			if !reflect.DeepEqual(cs.expectErr, err) {
				t.Errorf("expectErr: %v, but: %v", cs.expectErr, err)
			}
			if !reflect.DeepEqual(util.DumpJSON(cs.expect), util.DumpJSON(get)) {
				t.Errorf("expectGet: %v, but: %v", util.DumpJSON(cs.expect), util.DumpJSON(get))
			}
		})
	}
}
