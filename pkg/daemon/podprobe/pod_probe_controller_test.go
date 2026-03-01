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

package podprobe

import (
	"reflect"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
	listersalpha1 "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	commonutil "github.com/openkruise/kruise/pkg/util"
)

var (
	demoNodePodProbe = appsv1alpha1.NodePodProbe{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1alpha1.NodePodProbeSpec{
			PodProbes: []appsv1alpha1.PodProbe{
				{
					Name: "pod-0",
					UID:  "pod-0-uid",
					IP:   "1.1.1.1",
					Probes: []appsv1alpha1.ContainerProbe{
						{
							Name:          "ppm-1#healthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/healthy.sh"},
										},
									},
									InitialDelaySeconds: 100,
								},
							},
						},
					},
				},
				{
					Name: "pod-1",
					UID:  "pod-1-uid",
					IP:   "2.2.2.2",
					Probes: []appsv1alpha1.ContainerProbe{
						{
							Name:          "ppm-1#healthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/healthy.sh"},
										},
									},
									InitialDelaySeconds: 100,
								},
							},
						},
					},
				},
				{
					Name: "pod-2",
					UID:  "pod-2-uid",
					IP:   "3.3.3.3",
					Probes: []appsv1alpha1.ContainerProbe{
						{
							Name:          "ppm-1#tcpCheck",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8000)},
											Host: "3.3.3.3",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
)

func TestUpdateNodePodProbeStatus(t *testing.T) {
	cases := []struct {
		name                     string
		getUpdate                func() Update
		getNodePodProbe          func() *appsv1alpha1.NodePodProbe
		expectNodePodProbeStatus func() appsv1alpha1.NodePodProbeStatus
	}{
		{
			name: "test1, update pod probe status",
			getUpdate: func() Update {
				return Update{Key: probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "main", "ppm-1#healthy"}, State: appsv1alpha1.ProbeSucceeded}
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Status = appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-0",
							UID:  "pod-0-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbeStatus: func() appsv1alpha1.NodePodProbeStatus {
				obj := appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-0",
							UID:  "pod-0-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
					},
				}
				return obj
			},
		},

		{
			name: "test2, update pod probe status",
			getUpdate: func() Update {
				return Update{Key: probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "main", "ppm-1#healthy"}, State: appsv1alpha1.ProbeSucceeded}
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[1].Probes = append(demo.Spec.PodProbes[1].Probes, appsv1alpha1.ContainerProbe{
					Name:          "ppm-1#other",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/other.sh"},
								},
							},
							InitialDelaySeconds: 100,
						},
					},
				})
				demo.Status = appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-0",
							UID:  "pod-0-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#other",
									State: appsv1alpha1.ProbeFailed,
								},
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeFailed,
								},
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbeStatus: func() appsv1alpha1.NodePodProbeStatus {
				obj := appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-0",
							UID:  "pod-0-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#other",
									State: appsv1alpha1.ProbeFailed,
								},
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
					},
				}
				return obj
			},
		},

		{
			name: "test3, update pod probe status",
			getUpdate: func() Update {
				return Update{Key: probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "main", "ppm-1#healthy"}, State: appsv1alpha1.ProbeSucceeded}
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[1].Probes = append(demo.Spec.PodProbes[1].Probes, appsv1alpha1.ContainerProbe{
					Name:          "ppm-1#other",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/other.sh"},
								},
							},
							InitialDelaySeconds: 100,
						},
					},
				})
				demo.Status = appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#other",
									State: appsv1alpha1.ProbeFailed,
								},
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbeStatus: func() appsv1alpha1.NodePodProbeStatus {
				obj := appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#other",
									State: appsv1alpha1.ProbeFailed,
								},
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeSucceeded,
								},
							},
						},
					},
				}
				return obj
			},
		},

		{
			name: "test4, update pod probe status",
			getUpdate: func() Update {
				return Update{}
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Status = appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-2#other",
									State: appsv1alpha1.ProbeFailed,
								},
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeFailed,
								},
							},
						},
						{
							Name: "pod-2",
							UID:  "pod-2-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-2#other",
									State: appsv1alpha1.ProbeFailed,
								},
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbeStatus: func() appsv1alpha1.NodePodProbeStatus {
				return appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "ppm-1#healthy",
									State: appsv1alpha1.ProbeFailed,
								},
							},
						},
					},
				}
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset(cs.getNodePodProbe())
			informer := newNodePodProbeInformer(fakeClient, "node-1")
			fakeRecorder := record.NewFakeRecorder(100)
			c := &Controller{
				nodePodProbeInformer: informer,
				nodePodProbeLister:   listersalpha1.NewNodePodProbeLister(informer.GetIndexer()),
				workers:              make(map[probeKey]*worker),
				nodePodProbeClient:   fakeClient.AppsV1alpha1().NodePodProbes(),
				nodeName:             "node-1",
				result: newResultManager(workqueue.NewNamedRateLimitingQueue(
					workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second),
					"sync_node_pod_probe",
				)),
				eventRecorder: fakeRecorder,
			}
			stopCh := make(chan struct{}, 1)
			go c.nodePodProbeInformer.Run(stopCh)
			if !cache.WaitForCacheSync(stopCh, c.nodePodProbeInformer.HasSynced) {
				return
			}
			c.result.cache = &sync.Map{}
			c.result.cache.Store("container-id-1", cs.getUpdate())
			err := c.syncUpdateNodePodProbeStatus()
			if err != nil {
				t.Fatalf("syncUpdateNodePodProbeStatus failed: %s", err.Error())
				return
			}
			time.Sleep(time.Second)
			if !checkNodePodProbeStatusEqual(c.nodePodProbeLister, cs.expectNodePodProbeStatus()) {
				t.Fatalf("checkNodePodProbeStatusEqual failed")
			}
		})
	}
}

func checkNodePodProbeStatusEqual(lister listersalpha1.NodePodProbeLister, expect appsv1alpha1.NodePodProbeStatus) bool {
	npp, err := lister.Get("node-1")
	if err != nil {
		klog.ErrorS(err, "Get NodePodProbe failed")
		return false
	}
	for i := range npp.Status.PodProbeStatuses {
		podProbe := npp.Status.PodProbeStatuses[i]
		for j := range podProbe.ProbeStates {
			obj := &podProbe.ProbeStates[j]
			obj.LastTransitionTime = metav1.Time{}
			obj.LastProbeTime = metav1.Time{}
		}
	}
	return reflect.DeepEqual(npp.Status.PodProbeStatuses, expect.PodProbeStatuses)
}

func TestSyncNodePodProbe(t *testing.T) {
	cases := []struct {
		name            string
		getNodePodProbe func() *appsv1alpha1.NodePodProbe
		setWorkers      func(c *Controller)
		expectWorkers   func(c *Controller) map[probeKey]*worker
	}{
		{
			name: "test1, sync nodePodProbe",
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes = demo.Spec.PodProbes[1:]
				return demo
			},
			setWorkers: func(c *Controller) {
				c.workers = map[probeKey]*worker{}
				key1 := probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "main", "ppm-1#check"}
				c.workers[key1] = newWorker(c, key1, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/check.sh"},
							},
						},
					},
				})
				go c.workers[key1].run()
				key2 := probeKey{"", "pod-2", "pod-2-uid", "3.3.3.3", "main", "ppm-1#tcpCheck"}
				c.workers[key2] = newWorker(c, key2, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy2.sh"},
							},
						},
					},
				})
				go c.workers[key2].run()
			},
			expectWorkers: func(c *Controller) map[probeKey]*worker {
				expect := map[probeKey]*worker{}
				key1 := probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "main", "ppm-1#healthy"}
				expect[key1] = newWorker(c, key1, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy.sh"},
							},
						},
						InitialDelaySeconds: 100,
					},
				})
				key2 := probeKey{"", "pod-2", "pod-2-uid", "3.3.3.3", "main", "ppm-1#tcpCheck"}
				expect[key2] = newWorker(c, key2, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8000)},
								Host: "3.3.3.3",
							},
						},
					},
				})
				return expect
			},
		},
		{
			name: "test2, sync nodePodProbe",
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[1].Probes = append(demo.Spec.PodProbes[0].Probes, appsv1alpha1.ContainerProbe{
					Name:          "ppm-1#check",
					ContainerName: "nginx",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/check.sh"},
								},
							},
							InitialDelaySeconds: 100,
						},
					},
				})
				return demo
			},
			setWorkers: func(c *Controller) {
				c.workers = map[probeKey]*worker{}
			},
			expectWorkers: func(c *Controller) map[probeKey]*worker {
				expect := map[probeKey]*worker{}
				key0 := probeKey{"", "pod-0", "pod-0-uid", "1.1.1.1", "main", "ppm-1#healthy"}
				expect[key0] = newWorker(c, key0, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy.sh"},
							},
						},
						InitialDelaySeconds: 100,
					},
				})

				key1 := probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "main", "ppm-1#healthy"}
				expect[key1] = newWorker(c, key1, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy.sh"},
							},
						},
						InitialDelaySeconds: 100,
					},
				})

				key2 := probeKey{"", "pod-1", "pod-1-uid", "2.2.2.2", "nginx", "ppm-1#check"}
				expect[key2] = newWorker(c, key2, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/check.sh"},
							},
						},
						InitialDelaySeconds: 100,
					},
				})

				key3 := probeKey{"", "pod-2", "pod-2-uid", "3.3.3.3", "main", "ppm-1#tcpCheck"}
				expect[key3] = newWorker(c, key3, &appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8000)},
								Host: "3.3.3.3",
							},
						},
					},
				})
				return expect
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset(cs.getNodePodProbe())
			informer := newNodePodProbeInformer(fakeClient, "node-1")
			c := &Controller{
				nodePodProbeInformer: informer,
				nodePodProbeLister:   listersalpha1.NewNodePodProbeLister(informer.GetIndexer()),
				workers:              make(map[probeKey]*worker),
				nodePodProbeClient:   fakeClient.AppsV1alpha1().NodePodProbes(),
				nodeName:             "node-1",
				result: newResultManager(workqueue.NewNamedRateLimitingQueue(
					workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second),
					"sync_node_pod_probe",
				)),
				updateQueue: workqueue.NewNamedRateLimitingQueue(
					// Backoff duration from 500ms to 50~55s
					workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(1000)),
					"update_node_pod_probe_status",
				),
			}
			stopCh := make(chan struct{}, 1)
			go c.nodePodProbeInformer.Run(stopCh)
			if !cache.WaitForCacheSync(stopCh, c.nodePodProbeInformer.HasSynced) {
				return
			}
			cs.setWorkers(c)
			time.Sleep(time.Second)
			err := c.sync()
			if err != nil {
				t.Fatalf("NodePodProbe sync failed: %s", err.Error())
				return
			}
			time.Sleep(time.Second)

			c.workerLock.RLock()
			if len(c.workers) != len(cs.expectWorkers(c)) {
				t.Fatalf("expect(%d), but get(%d)", len(cs.expectWorkers(c)), len(c.workers))
			}
			c.workerLock.RUnlock()

			for _, worker := range cs.expectWorkers(c) {
				obj, ok := c.workers[worker.key]
				if !ok {
					t.Fatalf("expect(%v), but not found", worker.key)
				}
				if !reflect.DeepEqual(worker.spec, obj.spec) {
					t.Fatalf("expect(%s), but get(%s)", commonutil.DumpJSON(worker.spec), commonutil.DumpJSON(obj.spec))
				}
			}
		})
	}
}
