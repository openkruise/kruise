/*
Copyright 2020 The Kruise Authors.
Copyright 2015 The Kubernetes Authors.

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

package daemonset

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	kruisefake "github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
	kruiseinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions"
	kruiseappsinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/informers"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/controller"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/securitycontext"
	kubeClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	simpleDaemonSetLabel = map[string]string{"name": "simple-daemon", "type": "production"}
)

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}

const timeout = time.Second * 5

func newDaemonSet(name string) *appsv1alpha1.DaemonSet {
	two := int32(2)
	return &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			RevisionHistoryLimit: &two,
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.OnDeleteDaemonSetStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: simpleDaemonSetLabel},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: simpleDaemonSetLabel,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:                  "foo/bar",
							TerminationMessagePath: corev1.TerminationMessagePathDefault,
							ImagePullPolicy:        corev1.PullIfNotPresent,
							SecurityContext:        securitycontext.ValidSecurityContextWithContainerDefaults(),
						},
					},
					DNSPolicy: corev1.DNSDefault,
				},
			},
		},
	}
}

type fakePodControl struct {
	sync.Mutex
	*controller.FakePodControl
	podStore cache.Store
	podIDMap map[string]*corev1.Pod
	dsc      *DaemonSetsController
}

func newFakePodControl() *fakePodControl {
	podIDMap := make(map[string]*corev1.Pod)
	return &fakePodControl{
		FakePodControl: &controller.FakePodControl{},
		podIDMap:       podIDMap,
	}
}

func (f *fakePodControl) CreatePodsOnNode(nodeName, namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	f.Lock()
	defer f.Unlock()
	if err := f.FakePodControl.CreatePodsOnNode(nodeName, namespace, template, object, controllerRef); err != nil {
		return fmt.Errorf("failed to create pod on node %q", nodeName)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       template.Labels,
			Namespace:    namespace,
			GenerateName: fmt.Sprintf("%s-", nodeName),
		},
	}

	if err := legacyscheme.Scheme.Convert(&template.Spec, &pod.Spec, nil); err != nil {
		return fmt.Errorf("unable to convert pod template: %v", err)
	}
	if len(nodeName) != 0 {
		pod.Spec.NodeName = nodeName
	}
	pod.Name = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", nodeName))

	f.podStore.Update(pod)
	f.podIDMap[pod.Name] = pod

	ds := object.(*appsv1alpha1.DaemonSet)
	dsKey, _ := controller.KeyFunc(ds)
	expectations.CreationObserved(dsKey)

	return nil
}

func (f *fakePodControl) CreatePodsWithControllerRef(namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	f.Lock()
	defer f.Unlock()
	if err := f.FakePodControl.CreatePodsWithControllerRef(namespace, template, object, controllerRef); err != nil {
		return fmt.Errorf("failed to create pod for DaemonSet")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    template.Labels,
			Namespace: namespace,
		},
	}

	pod.Name = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%p-", pod))

	if err := legacyscheme.Scheme.Convert(&template.Spec, &pod.Spec, nil); err != nil {
		return fmt.Errorf("unable to convert pod template: %v", err)
	}

	f.podStore.Update(pod)
	f.podIDMap[pod.Name] = pod

	ds := object.(*apps.DaemonSet)
	dsKey, _ := controller.KeyFunc(ds)
	expectations.CreationObserved(dsKey)

	return nil
}

func (f *fakePodControl) DeletePod(namespace string, podID string, object runtime.Object) error {
	f.Lock()
	defer f.Unlock()
	if err := f.FakePodControl.DeletePod(namespace, podID, object); err != nil {
		return fmt.Errorf("failed to delete pod %q", podID)
	}
	pod, ok := f.podIDMap[podID]
	if !ok {
		return fmt.Errorf("pod %q does not exist", podID)
	}
	f.podStore.Delete(pod)
	delete(f.podIDMap, podID)

	ds := object.(*apps.DaemonSet)
	dsKey, _ := controller.KeyFunc(ds)
	expectations.DeletionObserved(dsKey)

	return nil
}

// just define for test
type DaemonSetsController struct {
	ReconcileDaemonSet
	// podListerSynced returns true if the pod shared informer has synced at least once
	podListerSynced cache.InformerSynced
	// setListerSynced returns true if the stateful set shared informer has synced at least once
	setListerSynced cache.InformerSynced
	// revListerSynced returns true if the rev shared informer has synced at least once
	revListerSynced cache.InformerSynced
}

func splitObjects(initialObjects []runtime.Object) ([]runtime.Object, []runtime.Object) {
	var kubeObjects []runtime.Object
	var kruiseObjects []runtime.Object
	for _, o := range initialObjects {
		if _, ok := o.(*appsv1alpha1.DaemonSet); ok {
			kruiseObjects = append(kruiseObjects, o)
		} else {
			kubeObjects = append(kubeObjects, o)
		}
	}
	return kubeObjects, kruiseObjects
}

func newTestController(c kubeClient.Client, initialObjects ...runtime.Object) (*DaemonSetsController, *fakePodControl, *fake.Clientset, error) {
	kubeObjects, kruiseObjects := splitObjects(initialObjects)
	client := fake.NewSimpleClientset(kubeObjects...)
	kruiseClient := kruisefake.NewSimpleClientset(kruiseObjects...)
	informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
	kruiseInformerFactory := kruiseinformers.NewSharedInformerFactory(kruiseClient, controller.NoResyncPeriodFunc())
	newDsc := NewDaemonSetController(
		informerFactory.Core().V1().Pods(),
		informerFactory.Core().V1().Nodes(),
		kruiseInformerFactory.Apps().V1alpha1().DaemonSets(),
		informerFactory.Apps().V1().ControllerRevisions(),
		client,
		c,
	)

	podControl := newFakePodControl()
	return newDsc, podControl, client, nil
}

func NewDaemonSetController(
	podInformer coreinformers.PodInformer,
	nodeInformer coreinformers.NodeInformer,
	setInformer kruiseappsinformers.DaemonSetInformer,
	revInformer appsinformers.ControllerRevisionInformer,
	kubeClient clientset.Interface,
	client kubeClient.Client,
) *DaemonSetsController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "daemonset-controller"})
	dsc := &DaemonSetsController{
		ReconcileDaemonSet: ReconcileDaemonSet{
			client:        client,
			eventRecorder: recorder,
			podControl:    kubecontroller.RealPodControl{KubeClient: kubeClient, Recorder: recorder},
			crControl: kubecontroller.RealControllerRevisionControl{
				KubeClient: kubeClient,
			},
			historyLister:       revInformer.Lister(),
			podLister:           podInformer.Lister(),
			nodeLister:          nodeInformer.Lister(),
			suspendedDaemonPods: map[string]sets.String{},
		},
		podListerSynced: podInformer.Informer().HasSynced,
		setListerSynced: setInformer.Informer().HasSynced,
		revListerSynced: revInformer.Informer().HasSynced,
	}
	return dsc
}

var c kubeClient.Client

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = util.NewClientFromManager(mgr, "test-daemonset-controller")
	err = client.NewRegistry(mgr.GetConfig())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	set := newDaemonSet("test")

	_, _, _, err = newTestController(c)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	dsc, err := newReconciler(mgr)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	recFn, requests := SetupTestReconcile(dsc)
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	err = c.Create(context.TODO(), set)
	if err != nil {
		t.Errorf("create DaemonSet %s failed: %s", set.Name, err)
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(time.Second * 2)
	foo := &appsv1alpha1.DaemonSet{}
	if err = c.Get(context.TODO(), types.NamespacedName{Namespace: set.Namespace, Name: set.Name}, foo); err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	err = c.Delete(context.TODO(), set)
	if err != nil {
		t.Errorf("delete DaemonSet %s failed: %s", set.Name, err)
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), set)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}

func Test_isControlledByDaemonSet(t *testing.T) {
	isController := true
	type args struct {
		p    *corev1.Pod
		uuid types.UID
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "isControlledByDaemonSet",
			args: args{
				p: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						OwnerReferences: []metav1.OwnerReference{
							{
								Controller: &isController,
								UID:        "111222333",
							},
						},
					},
				},
				uuid: "111222333",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isControlledByDaemonSet(tt.args.p, tt.args.uuid); got != tt.want {
				t.Errorf("isControlledByDaemonSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ignoreNotUnscheduable(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "not set annotation",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{},
				},
			},
			want: false,
		},
		{
			name: "set annotation false",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IsIgnoreNotReady: "false",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "set annotation true",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IsIgnoreNotReady: "true",
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ignoreNotUnscheduable(tt.args.ds); got != tt.want {
				t.Errorf("ignoreNotUnscheduable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ignoreNotReady(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "not set annotation",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{},
				},
			},
			want: false,
		},
		{
			name: "set annotation false",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IsIgnoreUnscheduable: "false",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "set annotation true",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IsIgnoreUnscheduable: "true",
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ignoreNotReady(tt.args.ds); got != tt.want {
				t.Errorf("ignoreNotReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanNodeBeDeployed(t *testing.T) {
	type args struct {
		node *corev1.Node
		ds   *appsv1alpha1.DaemonSet
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "ignoreAll",
			args: args{
				node: &corev1.Node{
					Spec: corev1.NodeSpec{
						Unschedulable: true,
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IsIgnoreUnscheduable: "true",
							IsIgnoreNotReady:     "true",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "respectAll",
			args: args{
				node: &corev1.Node{
					Spec: corev1.NodeSpec{
						Unschedulable: true,
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				ds: &appsv1alpha1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IsIgnoreUnscheduable: "false",
							IsIgnoreNotReady:     "false",
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanNodeBeDeployed(tt.args.node, tt.args.ds); got != tt.want {
				t.Errorf("CanNodeBeDeployed() = %v, want %v", got, tt.want)
			}
		})
	}
}
