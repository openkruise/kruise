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

package imagejob

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

var c client.Client
var cfg *rest.Config

func TestMain(m *testing.M) {
	runtime.GOMAXPROCS(4)
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
	}
	utilruntime.Must(apis.AddToScheme(clientgoscheme.Scheme))

	var err error
	if cfg, err = t.Start(); err != nil {
		klog.Fatal(err)
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}

// StartTestManager adds recFn
func StartTestManager(ctx context.Context, mgr manager.Manager, g *gomega.GomegaWithT) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(ctx)).NotTo(gomega.HaveOccurred())
	}()
	return wg
}

var (
	initialNodeImages = []*appsv1alpha1.NodeImage{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node2", Labels: map[string]string{"arch": "amd64"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node3", Labels: map[string]string{"arch": "arm64"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node4"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node5", Labels: map[string]string{"arch": "arm64"}},
		},
	}

	initialPods = []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Labels: map[string]string{"app": "foo"}},
			Spec:       v1.PodSpec{NodeName: "node2"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod2", Labels: map[string]string{"app": "bar"}},
			Spec:       v1.PodSpec{NodeName: "node1"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod3", Labels: map[string]string{"app": "foo"}},
			Spec:       v1.PodSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod4", Labels: map[string]string{}},
			Spec:       v1.PodSpec{NodeName: "node4"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod5", Labels: map[string]string{"app": "baz"}},
			Spec:       v1.PodSpec{NodeName: "node5"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod6", Namespace: "kube-system", Labels: map[string]string{"app": "foo"}},
			Spec:       v1.PodSpec{NodeName: "node5"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod7", Namespace: "kube-system", Labels: map[string]string{"app": "baz"}},
			Spec:       v1.PodSpec{NodeName: "node3"},
		},
	}

	initialJobs = []*appsv1alpha1.ImagePullJob{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job1"},
			Spec:       appsv1alpha1.ImagePullJobSpec{},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job2"},
			Spec: appsv1alpha1.ImagePullJobSpec{
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					Selector: &appsv1alpha1.ImagePullJobNodeSelector{Names: []string{"node2", "node4"}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job3"},
			Spec: appsv1alpha1.ImagePullJobSpec{
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					Selector: &appsv1alpha1.ImagePullJobNodeSelector{LabelSelector: metav1.LabelSelector{MatchLabels: map[string]string{"arch": "arm64"}}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job4"},
			Spec: appsv1alpha1.ImagePullJobSpec{
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					Selector: &appsv1alpha1.ImagePullJobNodeSelector{LabelSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "arch", Operator: metav1.LabelSelectorOpDoesNotExist}}}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job5"},
			Spec: appsv1alpha1.ImagePullJobSpec{
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					PodSelector: &appsv1alpha1.ImagePullJobPodSelector{LabelSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job6", Finalizers: []string{"apps.kruise.io/fake-block"}},
			Spec:       appsv1alpha1.ImagePullJobSpec{},
		},
	}
)

func TestAll(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	var err error
	mgr, err := manager.New(cfg, manager.Options{Metrics: metricsserver.Options{BindAddress: "0"}})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	_ = fieldindex.RegisterFieldIndexes(mgr.GetCache())
	c = utilclient.NewClientFromManager(mgr, "test-nodeimage-utils")

	for _, o := range initialNodeImages {
		if err = c.Create(context.TODO(), o); err != nil {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}
	for _, o := range initialPods {
		if o.Namespace == "" {
			o.Namespace = metav1.NamespaceDefault
		}
		o.Spec.Containers = []v1.Container{{Name: "fake", Image: "fake"}}
		if err = c.Create(context.TODO(), o); err != nil {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}
	for _, o := range initialJobs {
		if o.Namespace == "" {
			o.Namespace = metav1.NamespaceDefault
		}
		if err = c.Create(context.TODO(), o); err != nil {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}
		if len(o.Finalizers) > 0 {
			if err = c.Delete(context.TODO(), o); err != nil {
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	mgrStopped := StartTestManager(ctx, mgr, g)
	defer func() {
		cancel()
		mgrStopped.Wait()
	}()
	mgr.GetCache().WaitForCacheSync(ctx)

	// Test GetNodeImagesForJob
	testGetNodeImagesForJob(g)

	// Test GetActiveJobsForPod
	testGetActiveJobsForPod(g)

	// Test GetActiveJobsForNodeImage
	testGetActiveJobsForNodeImage(g)
}

func testGetNodeImagesForJob(g *gomega.GomegaWithT) {
	getNodeImagesForJob := func(job *appsv1alpha1.ImagePullJob) []string {
		nodeNames, err := GetNodeImagesForJob(c, job)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		names := sets.NewString()
		for _, n := range nodeNames {
			names.Insert(n.Name)
		}
		return names.List()
	}

	g.Expect(getNodeImagesForJob(initialJobs[0])).Should(gomega.Equal([]string{"node1", "node2", "node3", "node4", "node5"}))
	g.Expect(getNodeImagesForJob(initialJobs[1])).Should(gomega.Equal([]string{"node2", "node4"}))
	g.Expect(getNodeImagesForJob(initialJobs[2])).Should(gomega.Equal([]string{"node3", "node5"}))
	g.Expect(getNodeImagesForJob(initialJobs[3])).Should(gomega.Equal([]string{"node1", "node4"}))
	g.Expect(getNodeImagesForJob(initialJobs[4])).Should(gomega.Equal([]string{"node2"}))
	//g.Expect(getNodeImagesForJob(initialJobs[5])).Should(gomega.Equal([]string{"node2"}))
}

func testGetActiveJobsForPod(g *gomega.GomegaWithT) {
	getActiveJobsForPod := func(pod *v1.Pod) []string {
		jobs, _, err := GetActiveJobsForPod(c, pod, nil)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		names := sets.NewString()
		for _, j := range jobs {
			names.Insert(j.Name)
		}
		return names.List()
	}

	g.Expect(getActiveJobsForPod(initialPods[0])).Should(gomega.Equal([]string{"job5"}))
	g.Expect(getActiveJobsForPod(initialPods[1])).Should(gomega.Equal([]string{}))
	g.Expect(getActiveJobsForPod(initialPods[2])).Should(gomega.Equal([]string{"job5"}))
	g.Expect(getActiveJobsForPod(initialPods[3])).Should(gomega.Equal([]string{}))
	g.Expect(getActiveJobsForPod(initialPods[4])).Should(gomega.Equal([]string{}))
	g.Expect(getActiveJobsForPod(initialPods[5])).Should(gomega.Equal([]string{}))
	g.Expect(getActiveJobsForPod(initialPods[6])).Should(gomega.Equal([]string{}))
}

func testGetActiveJobsForNodeImage(g *gomega.GomegaWithT) {
	getActiveJobsForNodeImage := func(nodeImage *appsv1alpha1.NodeImage) []string {
		jobs, _, err := GetActiveJobsForNodeImage(c, nodeImage, nil)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		names := sets.NewString()
		for _, j := range jobs {
			names.Insert(j.Name)
		}
		return names.List()
	}

	g.Expect(getActiveJobsForNodeImage(initialNodeImages[0])).Should(gomega.Equal([]string{"job1", "job4"}))
	g.Expect(getActiveJobsForNodeImage(initialNodeImages[1])).Should(gomega.Equal([]string{"job1", "job2", "job5"}))
	g.Expect(getActiveJobsForNodeImage(initialNodeImages[2])).Should(gomega.Equal([]string{"job1", "job3"}))
	g.Expect(getActiveJobsForNodeImage(initialNodeImages[3])).Should(gomega.Equal([]string{"job1", "job2", "job4"}))
	g.Expect(getActiveJobsForNodeImage(initialNodeImages[4])).Should(gomega.Equal([]string{"job1", "job3", "job5"}))
}
