/*
Copyright 2026 The Kruise Authors.

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

package ephemeraljob

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kubeclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

// TestReconcileAgainstRealAPIServer runs Reconcile against an envtest
// apiserver. envtest has no kubelet, so pod status is set by the test.
func TestReconcileAgainstRealAPIServer(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest environment: %v", err)
	}
	defer func() {
		if err := testEnv.Stop(); err != nil {
			t.Logf("failed to stop envtest environment: %v", err)
		}
	}()

	// econtainer patches pods through the package-level generic clientset.
	if err := kubeclient.NewRegistry(cfg); err != nil {
		t.Fatalf("failed to init generic clientset registry: %v", err)
	}

	sch := runtime.NewScheme()
	if err := scheme.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := appsv1alpha1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add kruise scheme: %v", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: sch})
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	ctx := context.Background()
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ejob-it"}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	parallelism := int32(2)
	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed-outcome-job", Namespace: ns.Name},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "target"}},
			Parallelism: &parallelism,
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:  "debugger",
						Image: "busybox",
					}},
				},
			},
		},
	}
	if err := c.Create(ctx, job); err != nil {
		t.Fatalf("failed to create EphemeralJob: %v", err)
	}

	pods := []*v1.Pod{
		newRunnablePod("pod-succeed", ns.Name),
		newRunnablePod("pod-fail", ns.Name),
	}
	for _, p := range pods {
		if err := c.Create(ctx, p); err != nil {
			t.Fatalf("failed to create pod %s: %v", p.Name, err)
		}
	}
	for _, p := range pods {
		p.Status.Phase = v1.PodRunning
		p.Status.Conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}
		if err := c.Status().Update(ctx, p); err != nil {
			t.Fatalf("failed to set pod %s running: %v", p.Name, err)
		}
	}

	r := &ReconcileEphemeralJob{Client: c, scheme: sch, recorder: record.NewFakeRecorder(10)}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: job.Name, Namespace: job.Namespace}}

	// Injects the ephemeral containers into both pods.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	refreshed := &appsv1alpha1.EphemeralJob{}
	if err := c.Get(ctx, req.NamespacedName, refreshed); err != nil {
		t.Fatalf("failed to get job after first reconcile: %v", err)
	}
	t.Logf("phase before any pod reports status: %s", refreshed.Status.Phase)

	// Normally done by the pod watch handler once it observes the injection.
	key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}.String()
	scaleExpectations.ObserveScale(key, expectations.Create, "pod-fail-debugger")
	scaleExpectations.ObserveScale(key, expectations.Create, "pod-succeed-debugger")

	setEphemeralContainerTerminated(t, ctx, c, "pod-succeed", ns.Name, 0)
	setEphemeralContainerTerminated(t, ctx, c, "pod-fail", ns.Name, 1)

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	final := &appsv1alpha1.EphemeralJob{}
	if err := c.Get(ctx, req.NamespacedName, final); err != nil {
		t.Fatalf("failed to get job after second reconcile: %v", err)
	}

	t.Logf("final status: Matches=%d Succeeded=%d Failed=%d Running=%d Waiting=%d Phase=%s CompletionTime=%v",
		final.Status.Matches, final.Status.Succeeded, final.Status.Failed, final.Status.Running,
		final.Status.Waiting, final.Status.Phase, final.Status.CompletionTime)

	if final.Status.Phase != appsv1alpha1.EphemeralJobFailed {
		t.Errorf("expected phase Failed (mixed outcome, one pod failed), got %s", final.Status.Phase)
	}
	if final.Status.CompletionTime == nil {
		t.Errorf("expected CompletionTime to be set so TTL cleanup can run, got nil")
	}
	if final.Status.Succeeded != 1 || final.Status.Failed != 1 {
		t.Errorf("expected Succeeded=1 Failed=1, got Succeeded=%d Failed=%d", final.Status.Succeeded, final.Status.Failed)
	}
}

func newRunnablePod(name, namespace string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "target"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "main", Image: "busybox"}},
		},
	}
}

func setEphemeralContainerTerminated(t *testing.T, ctx context.Context, c client.Client, podName, namespace string, exitCode int32) {
	t.Helper()
	pod := &v1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, pod); err != nil {
		t.Fatalf("failed to get pod %s: %v", podName, err)
	}
	pod.Status.EphemeralContainerStatuses = []v1.ContainerStatus{
		{
			Name: "debugger",
			State: v1.ContainerState{
				Terminated: &v1.ContainerStateTerminated{
					ExitCode:   exitCode,
					FinishedAt: metav1.NewTime(time.Now()),
				},
			},
		},
	}
	if err := c.Status().Update(ctx, pod); err != nil {
		t.Fatalf("failed to patch ephemeral container status on pod %s: %v", podName, err)
	}
}
