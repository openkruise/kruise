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
	"sort"
	"sync"
	"testing"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/daemon/util"
	"k8s.io/kubernetes/pkg/securitycontext"
	testingclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	kruisefake "github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
	kruiseinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions"
	kruiseappsinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions/apps/v1alpha1"
	kruiseExpectations "github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
)

var (
	simpleDaemonSetLabel = map[string]string{"name": "simple-daemon", "type": "production"}
)

func init() {
	utilruntime.Must(apis.AddToScheme(scheme.Scheme))
}

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
	podStore     cache.Store
	podIDMap     map[string]*corev1.Pod
	expectations controller.ControllerExpectationsInterface
}

func newFakePodControl() *fakePodControl {
	podIDMap := make(map[string]*corev1.Pod)
	return &fakePodControl{
		FakePodControl: &controller.FakePodControl{},
		podIDMap:       podIDMap,
	}
}

func (f *fakePodControl) CreatePods(ctx context.Context, namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	f.Lock()
	defer f.Unlock()
	if err := f.FakePodControl.CreatePods(ctx, namespace, template, object, controllerRef); err != nil {
		return fmt.Errorf("failed to create pod for DaemonSet")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    template.Labels,
			Namespace: namespace,
		},
	}

	pod.Name = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%p-", pod))

	template.Spec.DeepCopyInto(&pod.Spec)

	f.podStore.Update(pod)
	f.podIDMap[pod.Name] = pod

	ds := object.(*appsv1alpha1.DaemonSet)
	dsKey, _ := controller.KeyFunc(ds)
	f.expectations.CreationObserved(klog.FromContext(ctx), dsKey)

	return nil
}

func (f *fakePodControl) DeletePod(ctx context.Context, namespace string, podID string, object runtime.Object) error {
	f.Lock()
	defer f.Unlock()
	if err := f.FakePodControl.DeletePod(ctx, namespace, podID, object); err != nil {
		return fmt.Errorf("failed to delete pod %q", podID)
	}
	pod, ok := f.podIDMap[podID]
	if !ok {
		return fmt.Errorf("pod %q does not exist", podID)
	}
	f.podStore.Delete(pod)
	delete(f.podIDMap, podID)

	ds := object.(*appsv1alpha1.DaemonSet)
	dsKey, _ := controller.KeyFunc(ds)
	f.expectations.DeletionObserved(klog.FromContext(ctx), dsKey)

	return nil
}

// just define for test
type daemonSetsController struct {
	*ReconcileDaemonSet
	dsStore      cache.Store
	historyStore cache.Store
	podStore     cache.Store
	nodeStore    cache.Store
	fakeRecorder *record.FakeRecorder
}

func (dsc *daemonSetsController) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	_, err = dsc.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}})
	return err
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

func newTestController(initialObjects ...runtime.Object) (*daemonSetsController, *fakePodControl, *fake.Clientset, error) {
	kubeObjects, kruiseObjects := splitObjects(initialObjects)
	client := fake.NewSimpleClientset(kubeObjects...)
	kruiseClient := kruisefake.NewSimpleClientset(kruiseObjects...)
	informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
	kruiseInformerFactory := kruiseinformers.NewSharedInformerFactory(kruiseClient, controller.NoResyncPeriodFunc())
	dsc := NewDaemonSetController(
		informerFactory.Core().V1().Pods(),
		informerFactory.Core().V1().Nodes(),
		kruiseInformerFactory.Apps().V1alpha1().DaemonSets(),
		informerFactory.Apps().V1().ControllerRevisions(),
		client,
		kruiseClient,
		flowcontrol.NewFakeBackOff(50*time.Millisecond, 500*time.Millisecond, testingclock.NewFakeClock(time.Now())),
	)

	fakeRecorder := record.NewFakeRecorder(100)
	dsc.eventRecorder = fakeRecorder
	podControl := newFakePodControl()
	dsc.podControl = podControl
	podControl.podStore = informerFactory.Core().V1().Pods().Informer().GetStore()

	newDsc := &daemonSetsController{
		dsc,
		kruiseInformerFactory.Apps().V1alpha1().DaemonSets().Informer().GetStore(),
		informerFactory.Apps().V1().ControllerRevisions().Informer().GetStore(),
		informerFactory.Core().V1().Pods().Informer().GetStore(),
		informerFactory.Core().V1().Nodes().Informer().GetStore(),
		fakeRecorder,
	}

	podControl.expectations = newDsc.expectations

	return newDsc, podControl, client, nil
}

func NewDaemonSetController(
	podInformer coreinformers.PodInformer,
	nodeInformer coreinformers.NodeInformer,
	dsInformer kruiseappsinformers.DaemonSetInformer,
	revInformer appsinformers.ControllerRevisionInformer,
	kubeClient clientset.Interface,
	kruiseClient kruiseclientset.Interface,
	failedPodsBackoff *flowcontrol.Backoff,
) *ReconcileDaemonSet {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "daemonset-controller"})
	return &ReconcileDaemonSet{
		kubeClient:    kubeClient,
		kruiseClient:  kruiseClient,
		eventRecorder: recorder,
		podControl:    controller.RealPodControl{KubeClient: kubeClient, Recorder: recorder},
		crControl: controller.RealControllerRevisionControl{
			KubeClient: kubeClient,
		},
		lifecycleControl:            lifecycle.NewForInformer(podInformer),
		expectations:                controller.NewControllerExpectations(),
		resourceVersionExpectations: kruiseExpectations.NewResourceVersionExpectation(),
		dsLister:                    dsInformer.Lister(),
		historyLister:               revInformer.Lister(),
		podLister:                   podInformer.Lister(),
		nodeLister:                  nodeInformer.Lister(),
		failedPodsBackoff:           failedPodsBackoff,
	}
}

func validateSyncDaemonSets(manager *daemonSetsController, fakePodControl *fakePodControl, expectedCreates, expectedDeletes int, expectedEvents int) error {
	if len(fakePodControl.Templates) != expectedCreates {
		return fmt.Errorf("Unexpected number of creates.  Expected %d, saw %d\n", expectedCreates, len(fakePodControl.Templates))
	}
	if len(fakePodControl.DeletePodName) != expectedDeletes {
		return fmt.Errorf("Unexpected number of deletes.  Expected %d, got %v\n", expectedDeletes, fakePodControl.DeletePodName)
	}
	if len(manager.fakeRecorder.Events) != expectedEvents {
		return fmt.Errorf("Unexpected number of events.  Expected %d, saw %d\n", expectedEvents, len(manager.fakeRecorder.Events))
	}
	// Every Pod created should have a ControllerRef.
	if got, want := len(fakePodControl.ControllerRefs), expectedCreates; got != want {
		return fmt.Errorf("len(ControllerRefs) = %v, want %v", got, want)
	}
	// Make sure the ControllerRefs are correct.
	for _, controllerRef := range fakePodControl.ControllerRefs {
		if got, want := controllerRef.APIVersion, "apps.kruise.io/v1alpha1"; got != want {
			return fmt.Errorf("controllerRef.APIVersion = %q, want %q", got, want)
		}
		if got, want := controllerRef.Kind, "DaemonSet"; got != want {
			return fmt.Errorf("controllerRef.Kind = %q, want %q", got, want)
		}
		if controllerRef.Controller == nil || *controllerRef.Controller != true {
			return fmt.Errorf("controllerRef.Controller is not set to true")
		}
	}
	return nil
}

func expectSyncDaemonSets(t *testing.T, manager *daemonSetsController, ds *appsv1alpha1.DaemonSet, podControl *fakePodControl, expectedCreates, expectedDeletes int, expectedEvents int) {
	t.Helper()
	key, err := controller.KeyFunc(ds)
	if err != nil {
		t.Fatal("could not get key for daemon")
	}

	err = manager.syncHandler(key)
	if err != nil {
		t.Log(err)
	}

	err = validateSyncDaemonSets(manager, podControl, expectedCreates, expectedDeletes, expectedEvents)
	if err != nil {
		t.Fatal(err)
	}
}

func markPodsReady(store cache.Store) {
	// mark pods as ready
	for _, obj := range store.List() {
		pod := obj.(*corev1.Pod)
		markPodReady(pod)
	}
}

func markPodReady(pod *corev1.Pod) {
	condition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue}
	podutil.UpdatePodCondition(&pod.Status, &condition)
}

// clearExpectations copies the FakePodControl to PodStore and clears the create and delete expectations.
func clearExpectations(t *testing.T, manager *daemonSetsController, ds *appsv1alpha1.DaemonSet, fakePodControl *fakePodControl) {
	fakePodControl.Clear()

	key, err := controller.KeyFunc(ds)
	if err != nil {
		t.Errorf("Could not get key for daemon.")
		return
	}
	manager.expectations.DeleteExpectations(klog.FromContext(context.TODO()), key)

	now := manager.failedPodsBackoff.Clock.Now()
	hash, _ := currentDSHash(context.TODO(), manager, ds)
	// log all the pods in the store
	var lines []string
	for _, obj := range manager.podStore.List() {
		pod := obj.(*corev1.Pod)
		if pod.CreationTimestamp.IsZero() {
			pod.CreationTimestamp.Time = now
		}
		var readyLast time.Time
		ready := podutil.IsPodReady(pod)
		if ready {
			if c := podutil.GetPodReadyCondition(pod.Status); c != nil {
				readyLast = c.LastTransitionTime.Time.Add(time.Duration(ds.Spec.MinReadySeconds) * time.Second)
			}
		}
		nodeName, _ := util.GetTargetNodeName(pod)

		lines = append(lines, fmt.Sprintf("node=%s current=%-5t ready=%-5t age=%-4d pod=%s now=%d available=%d",
			nodeName,
			hash == pod.Labels[apps.ControllerRevisionHashLabelKey],
			ready,
			now.Unix(),
			pod.Name,
			pod.CreationTimestamp.Unix(),
			readyLast.Unix(),
		))
	}
	sort.Strings(lines)
	for _, line := range lines {
		klog.InfoS(line)
	}
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
