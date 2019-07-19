/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	kruisefake "github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
	kruiseinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions"
	kruiseappsinformers "github.com/openkruise/kruise/pkg/client/informers/externalversions/apps/v1alpha1"
	kruiseappslisters "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const statefulSetResyncPeriod = 30 * time.Second

func alwaysReady() bool { return true }

func TestStatefulSetControllerCreates(t *testing.T) {

	set := newStatefulSet(3)
	ssc, spc := newFakeStatefulSetController(set)
	if err := scaleUpStatefulSetController(set, ssc, spc); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	if obj, _, err := spc.setsIndexer.Get(set); err != nil {
		t.Error(err)
	} else {
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	if set.Status.Replicas != 3 {
		t.Errorf("set.Status.Replicas = %v; want 3", set.Status.Replicas)
	}
}

func TestStatefulSetControllerDeletes(t *testing.T) {
	set := newStatefulSet(3)
	ssc, spc := newFakeStatefulSetController(set)
	if err := scaleUpStatefulSetController(set, ssc, spc); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	if obj, _, err := spc.setsIndexer.Get(set); err != nil {
		t.Error(err)
	} else {
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	if set.Status.Replicas != 3 {
		t.Errorf("set.Status.Replicas = %v; want 3", set.Status.Replicas)
	}
	*set.Spec.Replicas = 0
	if err := scaleDownStatefulSetController(set, ssc, spc); err != nil {
		t.Errorf("Failed to turn down StatefulSet : %s", err)
	}
	if obj, _, err := spc.setsIndexer.Get(set); err != nil {
		t.Error(err)
	} else {
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	if set.Status.Replicas != 0 {
		t.Errorf("set.Status.Replicas = %v; want 0", set.Status.Replicas)
	}
}

func TestStatefulSetControllerRespectsTermination(t *testing.T) {
	set := newStatefulSet(3)
	ssc, spc := newFakeStatefulSetController(set)
	if err := scaleUpStatefulSetController(set, ssc, spc); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	if obj, _, err := spc.setsIndexer.Get(set); err != nil {
		t.Error(err)
	} else {
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	if set.Status.Replicas != 3 {
		t.Errorf("set.Status.Replicas = %v; want 3", set.Status.Replicas)
	}
	_, err := spc.addTerminatingPod(set, 3)
	if err != nil {
		t.Error(err)
	}
	pods, err := spc.addTerminatingPod(set, 4)
	if err != nil {
		t.Error(err)
	}
	ssc.syncStatefulSet(set, pods)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Error(err)
	}
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if len(pods) != 5 {
		t.Error("StatefulSet does not respect termination")
	}
	sort.Sort(ascendingOrdinal(pods))
	spc.DeleteStatefulPod(set, pods[3])
	spc.DeleteStatefulPod(set, pods[4])
	*set.Spec.Replicas = 0
	if err := scaleDownStatefulSetController(set, ssc, spc); err != nil {
		t.Errorf("Failed to turn down StatefulSet : %s", err)
	}
	if obj, _, err := spc.setsIndexer.Get(set); err != nil {
		t.Error(err)
	} else {
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	if set.Status.Replicas != 0 {
		t.Errorf("set.Status.Replicas = %v; want 0", set.Status.Replicas)
	}
}

func TestStatefulSetControllerBlocksScaling(t *testing.T) {
	set := newStatefulSet(3)
	ssc, spc := newFakeStatefulSetController(set)
	if err := scaleUpStatefulSetController(set, ssc, spc); err != nil {
		t.Errorf("Failed to turn up StatefulSet : %s", err)
	}
	if obj, _, err := spc.setsIndexer.Get(set); err != nil {
		t.Error(err)
	} else {
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	if set.Status.Replicas != 3 {
		t.Errorf("set.Status.Replicas = %v; want 3", set.Status.Replicas)
	}
	*set.Spec.Replicas = 5
	fakeResourceVersion(set)
	spc.setsIndexer.Update(set)
	_, err := spc.setPodTerminated(set, 0)
	if err != nil {
		t.Error("Failed to set pod terminated at ordinal 0")
	}
	ssc.enqueueStatefulSet(set)
	fakeWorker(ssc)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Error(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if len(pods) != 3 {
		t.Error("StatefulSet does not block scaling")
	}
	sort.Sort(ascendingOrdinal(pods))
	spc.DeleteStatefulPod(set, pods[0])
	ssc.enqueueStatefulSet(set)
	fakeWorker(ssc)
	pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Error(err)
	}
	if len(pods) != 3 {
		t.Error("StatefulSet does not resume when terminated Pod is removed")
	}
}

func TestStatefulSetControllerDeletionTimestamp(t *testing.T) {
	set := newStatefulSet(3)
	set.DeletionTimestamp = new(metav1.Time)
	ssc, spc := newFakeStatefulSetController(set)

	spc.setsIndexer.Add(set)

	// Force a sync. It should not try to create any Pods.
	ssc.enqueueStatefulSet(set)
	fakeWorker(ssc)

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(pods), 0; got != want {
		t.Errorf("len(pods) = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerDeletionTimestampRace(t *testing.T) {
	set := newStatefulSet(3)
	// The bare client says it IS deleted.
	set.DeletionTimestamp = new(metav1.Time)
	ssc, spc := newFakeStatefulSetController(set)

	// The lister (cache) says it's NOT deleted.
	set2 := *set
	set2.DeletionTimestamp = nil
	spc.setsIndexer.Add(&set2)

	// The recheck occurs in the presence of a matching orphan.
	pod := newStatefulSetPod(set, 1)
	pod.OwnerReferences = nil
	spc.podsIndexer.Add(pod)

	// Force a sync. It should not try to create any Pods.
	ssc.enqueueStatefulSet(set)
	fakeWorker(ssc)

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(pods), 1; got != want {
		t.Errorf("len(pods) = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerAddPod(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set1 := newStatefulSet(3)
	set2 := newStatefulSet(3)
	pod1 := newStatefulSetPod(set1, 0)
	pod2 := newStatefulSetPod(set2, 0)
	spc.setsIndexer.Add(set1)
	spc.setsIndexer.Add(set2)

	ssc.addPod(pod1)
	key, done := ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set1); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}
	ssc.queue.Done(key)

	ssc.addPod(pod2)
	key, done = ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set2); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}
	ssc.queue.Done(key)
}

func TestStatefulSetControllerAddPodOrphan(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set1 := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	set3 := newStatefulSet(3)
	set3.Name = "foo3"
	set3.Spec.Selector.MatchLabels = map[string]string{"foo3": "bar"}
	pod := newStatefulSetPod(set1, 0)
	spc.setsIndexer.Add(set1)
	spc.setsIndexer.Add(set2)
	spc.setsIndexer.Add(set3)

	// Make pod an orphan. Expect matching sets to be queued.
	pod.OwnerReferences = nil
	ssc.addPod(pod)
	if got, want := ssc.queue.Len(), 2; got != want {
		t.Errorf("queue.Len() = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerAddPodNoSet(t *testing.T) {
	ssc, _ := newFakeStatefulSetController()
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	ssc.addPod(pod)
	ssc.queue.ShutDown()
	key, _ := ssc.queue.Get()
	if key != nil {
		t.Errorf("StatefulSet enqueued key for Pod with no Set %s", key)
	}
}

func TestStatefulSetControllerUpdatePod(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set1 := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod1 := newStatefulSetPod(set1, 0)
	pod2 := newStatefulSetPod(set2, 0)
	spc.setsIndexer.Add(set1)
	spc.setsIndexer.Add(set2)

	prev := *pod1
	fakeResourceVersion(pod1)
	ssc.updatePod(&prev, pod1)
	key, done := ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set1); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}

	prev = *pod2
	fakeResourceVersion(pod2)
	ssc.updatePod(&prev, pod2)
	key, done = ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set2); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}
}

func TestStatefulSetControllerUpdatePodWithNoSet(t *testing.T) {
	ssc, _ := newFakeStatefulSetController()
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	prev := *pod
	fakeResourceVersion(pod)
	ssc.updatePod(&prev, pod)
	ssc.queue.ShutDown()
	key, _ := ssc.queue.Get()
	if key != nil {
		t.Errorf("StatefulSet enqueued key for Pod with no Set %s", key)
	}
}

func TestStatefulSetControllerUpdatePodWithSameVersion(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	spc.setsIndexer.Add(set)
	ssc.updatePod(pod, pod)
	ssc.queue.ShutDown()
	key, _ := ssc.queue.Get()
	if key != nil {
		t.Errorf("StatefulSet enqueued key for Pod with no Set %s", key)
	}
}

func TestStatefulSetControllerUpdatePodOrphanWithNewLabels(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	pod.OwnerReferences = nil
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	spc.setsIndexer.Add(set)
	spc.setsIndexer.Add(set2)
	clone := *pod
	clone.Labels = map[string]string{"foo2": "bar2"}
	fakeResourceVersion(&clone)
	ssc.updatePod(&clone, pod)
	if got, want := ssc.queue.Len(), 2; got != want {
		t.Errorf("queue.Len() = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerUpdatePodChangeControllerRef(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod := newStatefulSetPod(set, 0)
	pod2 := newStatefulSetPod(set2, 0)
	spc.setsIndexer.Add(set)
	spc.setsIndexer.Add(set2)
	clone := *pod
	clone.OwnerReferences = pod2.OwnerReferences
	fakeResourceVersion(&clone)
	ssc.updatePod(&clone, pod)
	if got, want := ssc.queue.Len(), 2; got != want {
		t.Errorf("queue.Len() = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerUpdatePodRelease(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod := newStatefulSetPod(set, 0)
	spc.setsIndexer.Add(set)
	spc.setsIndexer.Add(set2)
	clone := *pod
	clone.OwnerReferences = nil
	fakeResourceVersion(&clone)
	ssc.updatePod(pod, &clone)
	if got, want := ssc.queue.Len(), 2; got != want {
		t.Errorf("queue.Len() = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerDeletePod(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set1 := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod1 := newStatefulSetPod(set1, 0)
	pod2 := newStatefulSetPod(set2, 0)
	spc.setsIndexer.Add(set1)
	spc.setsIndexer.Add(set2)

	ssc.deletePod(pod1)
	key, done := ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set1); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}

	ssc.deletePod(pod2)
	key, done = ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set2); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}
}

func TestStatefulSetControllerDeletePodOrphan(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set1 := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod1 := newStatefulSetPod(set1, 0)
	spc.setsIndexer.Add(set1)
	spc.setsIndexer.Add(set2)

	pod1.OwnerReferences = nil
	ssc.deletePod(pod1)
	if got, want := ssc.queue.Len(), 0; got != want {
		t.Errorf("queue.Len() = %v, want %v", got, want)
	}
}

func TestStatefulSetControllerDeletePodTombstone(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	spc.setsIndexer.Add(set)
	tombstoneKey, _ := controller.KeyFunc(pod)
	tombstone := cache.DeletedFinalStateUnknown{Key: tombstoneKey, Obj: pod}
	ssc.deletePod(tombstone)
	key, done := ssc.queue.Get()
	if key == nil || done {
		t.Error("failed to enqueue StatefulSet")
	} else if key, ok := key.(string); !ok {
		t.Error("key is not a string")
	} else if expectedKey, _ := controller.KeyFunc(set); expectedKey != key {
		t.Errorf("expected StatefulSet key %s found %s", expectedKey, key)
	}
}

func TestStatefulSetControllerGetStatefulSetsForPod(t *testing.T) {
	ssc, spc := newFakeStatefulSetController()
	set1 := newStatefulSet(3)
	set2 := newStatefulSet(3)
	set2.Name = "foo2"
	pod := newStatefulSetPod(set1, 0)
	spc.setsIndexer.Add(set1)
	spc.setsIndexer.Add(set2)
	spc.podsIndexer.Add(pod)
	sets := ssc.getStatefulSetsForPod(pod)
	if got, want := len(sets), 2; got != want {
		t.Errorf("len(sets) = %v, want %v", got, want)
	}
}

func TestGetPodsForStatefulSetAdopt(t *testing.T) {
	set := newStatefulSet(5)
	pod1 := newStatefulSetPod(set, 1)
	// pod2 is an orphan with matching labels and name.
	pod2 := newStatefulSetPod(set, 2)
	pod2.OwnerReferences = nil
	// pod3 has wrong labels.
	pod3 := newStatefulSetPod(set, 3)
	pod3.OwnerReferences = nil
	pod3.Labels = nil
	// pod4 has wrong name.
	pod4 := newStatefulSetPod(set, 4)
	pod4.OwnerReferences = nil
	pod4.Name = "x" + pod4.Name

	ssc, spc := newFakeStatefulSetController(set, pod1, pod2, pod3, pod4)

	spc.podsIndexer.Add(pod1)
	spc.podsIndexer.Add(pod2)
	spc.podsIndexer.Add(pod3)
	spc.podsIndexer.Add(pod4)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	pods, err := ssc.getPodsForStatefulSet(set, selector)
	if err != nil {
		t.Fatalf("getPodsForStatefulSet() error: %v", err)
	}
	got := sets.NewString()
	for _, pod := range pods {
		got.Insert(pod.Name)
	}
	// pod2 should be claimed, pod3 and pod4 ignored
	want := sets.NewString(pod1.Name, pod2.Name)
	if !got.Equal(want) {
		t.Errorf("getPodsForStatefulSet() = %v, want %v", got, want)
	}
}

func TestGetPodsForStatefulSetRelease(t *testing.T) {
	set := newStatefulSet(3)
	ssc, spc := newFakeStatefulSetController(set)
	pod1 := newStatefulSetPod(set, 1)
	// pod2 is owned but has wrong name.
	pod2 := newStatefulSetPod(set, 2)
	pod2.Name = "x" + pod2.Name
	// pod3 is owned but has wrong labels.
	pod3 := newStatefulSetPod(set, 3)
	pod3.Labels = nil
	// pod4 is an orphan that doesn't match.
	pod4 := newStatefulSetPod(set, 4)
	pod4.OwnerReferences = nil
	pod4.Labels = nil

	spc.podsIndexer.Add(pod1)
	spc.podsIndexer.Add(pod2)
	spc.podsIndexer.Add(pod3)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		t.Fatal(err)
	}
	pods, err := ssc.getPodsForStatefulSet(set, selector)
	if err != nil {
		t.Fatalf("getPodsForStatefulSet() error: %v", err)
	}
	got := sets.NewString()
	for _, pod := range pods {
		got.Insert(pod.Name)
	}

	// Expect only pod1 (pod2 and pod3 should be released, pod4 ignored).
	want := sets.NewString(pod1.Name)
	if !got.Equal(want) {
		t.Errorf("getPodsForStatefulSet() = %v, want %v", got, want)
	}
}

func splitObjects(initialObjects []runtime.Object) ([]runtime.Object, []runtime.Object) {
	var kubeObjects []runtime.Object
	var kruiseObjects []runtime.Object
	for _, o := range initialObjects {
		if _, ok := o.(*appsv1alpha1.StatefulSet); ok {
			kruiseObjects = append(kruiseObjects, o)
		} else {
			kubeObjects = append(kubeObjects, o)
		}
	}
	return kubeObjects, kruiseObjects
}

func newFakeStatefulSetController(initialObjects ...runtime.Object) (*StatefulSetController, *fakeStatefulPodControl) {
	kubeObjects, kruiseObjects := splitObjects(initialObjects)
	client := fake.NewSimpleClientset(kubeObjects...)
	kruiseClient := kruisefake.NewSimpleClientset(kruiseObjects...)
	informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
	kruiseInformerFactory := kruiseinformers.NewSharedInformerFactory(kruiseClient, controller.NoResyncPeriodFunc())
	fpc := newFakeStatefulPodControl(informerFactory.Core().V1().Pods(), kruiseInformerFactory.Apps().V1alpha1().StatefulSets())
	ssu := newFakeStatefulSetStatusUpdater(kruiseInformerFactory.Apps().V1alpha1().StatefulSets())
	ssc := NewStatefulSetController(
		informerFactory.Core().V1().Pods(),
		kruiseInformerFactory.Apps().V1alpha1().StatefulSets(),
		informerFactory.Core().V1().PersistentVolumeClaims(),
		informerFactory.Apps().V1().ControllerRevisions(),
		client,
		kruiseClient,
	)
	ssh := history.NewFakeHistory(informerFactory.Apps().V1().ControllerRevisions())
	ssc.podListerSynced = alwaysReady
	ssc.setListerSynced = alwaysReady
	recorder := record.NewFakeRecorder(10)
	ssc.control = NewDefaultStatefulSetControl(fpc, ssu, ssh, recorder)

	return ssc, fpc
}

func fakeWorker(ssc *StatefulSetController) {
	if obj, done := ssc.queue.Get(); !done {
		ssc.sync(obj.(string))
		ssc.queue.Done(obj)
	}
}

func getPodAtOrdinal(pods []*v1.Pod, ordinal int) *v1.Pod {
	if 0 > ordinal || ordinal >= len(pods) {
		return nil
	}
	sort.Sort(ascendingOrdinal(pods))
	return pods[ordinal]
}

func scaleUpStatefulSetController(set *appsv1alpha1.StatefulSet, ssc *StatefulSetController, spc *fakeStatefulPodControl) error {
	spc.setsIndexer.Add(set)
	ssc.enqueueStatefulSet(set)
	fakeWorker(ssc)
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	for set.Status.ReadyReplicas < *set.Spec.Replicas {
		pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			return err
		}
		ord := len(pods) - 1
		if pods, err = spc.setPodPending(set, ord); err != nil {
			return err
		}
		pod := getPodAtOrdinal(pods, ord)
		ssc.addPod(pod)
		fakeWorker(ssc)
		pod = getPodAtOrdinal(pods, ord)
		prev := *pod
		if pods, err = spc.setPodRunning(set, ord); err != nil {
			return err
		}
		pod = getPodAtOrdinal(pods, ord)
		ssc.updatePod(&prev, pod)
		fakeWorker(ssc)
		pod = getPodAtOrdinal(pods, ord)
		prev = *pod
		if pods, err = spc.setPodReady(set, ord); err != nil {
			return err
		}
		pod = getPodAtOrdinal(pods, ord)
		ssc.updatePod(&prev, pod)
		fakeWorker(ssc)
		if err := assertMonotonicInvariants(set, spc); err != nil {
			return err
		}
		obj, _, err := spc.setsIndexer.Get(set)
		if err != nil {
			return err
		}
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	return assertMonotonicInvariants(set, spc)
}

func scaleDownStatefulSetController(set *appsv1alpha1.StatefulSet, ssc *StatefulSetController, spc *fakeStatefulPodControl) error {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return err
	}
	pods, err := spc.podsLister.Pods(set.Namespace).List(selector)
	if err != nil {
		return err
	}
	ord := len(pods) - 1
	pod := getPodAtOrdinal(pods, ord)
	prev := *pod
	fakeResourceVersion(set)
	spc.setsIndexer.Add(set)
	ssc.enqueueStatefulSet(set)
	fakeWorker(ssc)
	pods, err = spc.addTerminatingPod(set, ord)
	if err != nil {
		return err
	}
	pod = getPodAtOrdinal(pods, ord)
	ssc.updatePod(&prev, pod)
	fakeWorker(ssc)
	spc.DeleteStatefulPod(set, pod)
	ssc.deletePod(pod)
	fakeWorker(ssc)
	for set.Status.Replicas > *set.Spec.Replicas {
		pods, err = spc.podsLister.Pods(set.Namespace).List(selector)
		if err != nil {
			return err
		}

		ord := len(pods)
		pods, err = spc.addTerminatingPod(set, ord)
		if err != nil {
			return err
		}
		pod = getPodAtOrdinal(pods, ord)
		ssc.updatePod(&prev, pod)
		fakeWorker(ssc)
		spc.DeleteStatefulPod(set, pod)
		ssc.deletePod(pod)
		fakeWorker(ssc)
		obj, _, err := spc.setsIndexer.Get(set)
		if err != nil {
			return err
		}
		set = obj.(*appsv1alpha1.StatefulSet)
	}
	return assertMonotonicInvariants(set, spc)
}

// just define for test
type StatefulSetController struct {
	ReconcileStatefulSet
	// podListerSynced returns true if the pod shared informer has synced at least once
	podListerSynced cache.InformerSynced
	// setListerSynced returns true if the stateful set shared informer has synced at least once
	setListerSynced cache.InformerSynced
	// pvcListerSynced returns true if the pvc shared informer has synced at least once
	pvcListerSynced cache.InformerSynced
	// revListerSynced returns true if the rev shared informer has synced at least once
	revListerSynced cache.InformerSynced
	// StatefulSets that need to be synced.
	queue workqueue.RateLimitingInterface
}

func NewStatefulSetController(
	podInformer coreinformers.PodInformer,
	setInformer kruiseappsinformers.StatefulSetInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	revInformer appsinformers.ControllerRevisionInformer,
	kubeClient clientset.Interface,
	kruiseClient kruiseclientset.Interface,
) *StatefulSetController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "statefulset-controller"})

	ssc := &StatefulSetController{
		ReconcileStatefulSet: ReconcileStatefulSet{
			kruiseClient: kruiseClient,
			control: NewDefaultStatefulSetControl(
				NewRealStatefulPodControl(
					kubeClient,
					setInformer.Lister(),
					podInformer.Lister(),
					pvcInformer.Lister(),
					recorder),
				NewRealStatefulSetStatusUpdater(kruiseClient, setInformer.Lister()),
				history.NewHistory(kubeClient, revInformer.Lister()),
				recorder,
			),
			podControl: kubecontroller.RealPodControl{KubeClient: kubeClient, Recorder: recorder},
			podLister:  podInformer.Lister(),
			setLister:  setInformer.Lister(),
		},
		podListerSynced: podInformer.Informer().HasSynced,
		setListerSynced: setInformer.Informer().HasSynced,
		revListerSynced: revInformer.Informer().HasSynced,
		pvcListerSynced: pvcInformer.Informer().HasSynced,
		queue:           workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "statefulset"),
	}

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// lookup the statefulset and enqueue
		AddFunc: ssc.addPod,
		// lookup current and old statefulset if labels changed
		UpdateFunc: ssc.updatePod,
		// lookup statefulset accounting for deletion tombstones
		DeleteFunc: ssc.deletePod,
	})

	setInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: ssc.enqueueStatefulSet,
			UpdateFunc: func(old, cur interface{}) {
				oldPS := old.(*appsv1alpha1.StatefulSet)
				curPS := cur.(*appsv1alpha1.StatefulSet)
				if oldPS.Status.Replicas != curPS.Status.Replicas {
					klog.V(4).Infof("Observed updated replica count for StatefulSet: %v, %d->%d", curPS.Name, oldPS.Status.Replicas, curPS.Status.Replicas)
				}
				ssc.enqueueStatefulSet(cur)
			},
			DeleteFunc: ssc.enqueueStatefulSet,
		},
		statefulSetResyncPeriod,
	)

	return ssc
}

// Run runs the statefulset controller.
func (ssc *StatefulSetController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer ssc.queue.ShutDown()

	klog.Infof("Starting stateful set controller")
	defer klog.Infof("Shutting down statefulset controller")

	if !controller.WaitForCacheSync("stateful set", stopCh, ssc.podListerSynced, ssc.setListerSynced, ssc.pvcListerSynced, ssc.revListerSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(ssc.worker, time.Second, stopCh)
	}

	<-stopCh
}

// worker runs a worker goroutine that invokes processNextWorkItem until the controller's queue is closed
func (ssc *StatefulSetController) worker() {
	for ssc.processNextWorkItem() {
	}
}

// enqueueStatefulSet enqueues the given statefulset in the work queue.
func (ssc *StatefulSetController) enqueueStatefulSet(obj interface{}) {
	key, err := controller.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %+v: %v", obj, err))
		return
	}
	ssc.queue.Add(key)
}

// processNextWorkItem dequeues items, processes them, and marks them done. It enforces that the syncHandler is never
// invoked concurrently with the same key.
func (ssc *StatefulSetController) processNextWorkItem() bool {
	key, quit := ssc.queue.Get()
	if quit {
		return false
	}
	defer ssc.queue.Done(key)
	if err := ssc.sync(key.(string)); err != nil {
		utilruntime.HandleError(fmt.Errorf("Error syncing StatefulSet %v, requeuing: %v", key.(string), err))
		ssc.queue.AddRateLimited(key)
	} else {
		ssc.queue.Forget(key)
	}
	return true
}

func (ssc *StatefulSetController) sync(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	_, err = ssc.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}})
	return err
}

// addPod adds the statefulset for the pod to the sync queue
func (ssc *StatefulSetController) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)

	if pod.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		ssc.deletePod(pod)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		set := ssc.resolveControllerRef(pod.Namespace, controllerRef)
		if set == nil {
			return
		}
		klog.V(4).Infof("Pod %s created, labels: %+v", pod.Name, pod.Labels)
		ssc.enqueueStatefulSet(set)
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching controllers and sync
	// them to see if anyone wants to adopt it.
	sets := ssc.getStatefulSetsForPod(pod)
	if len(sets) == 0 {
		return
	}
	klog.V(4).Infof("Orphan Pod %s created, labels: %+v", pod.Name, pod.Labels)
	for _, set := range sets {
		ssc.enqueueStatefulSet(set)
	}
}

// updatePod adds the statefulset for the current and old pods to the sync queue.
func (ssc *StatefulSetController) updatePod(old, cur interface{}) {
	curPod := cur.(*v1.Pod)
	oldPod := old.(*v1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		// Periodic resync will send update events for all known pods.
		// Two different versions of the same pod will always have different RVs.
		return
	}

	labelChanged := !reflect.DeepEqual(curPod.Labels, oldPod.Labels)

	curControllerRef := metav1.GetControllerOf(curPod)
	oldControllerRef := metav1.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if set := ssc.resolveControllerRef(oldPod.Namespace, oldControllerRef); set != nil {
			ssc.enqueueStatefulSet(set)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		set := ssc.resolveControllerRef(curPod.Namespace, curControllerRef)
		if set == nil {
			return
		}
		klog.V(4).Infof("Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		ssc.enqueueStatefulSet(set)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	if labelChanged || controllerRefChanged {
		sets := ssc.getStatefulSetsForPod(curPod)
		if len(sets) == 0 {
			return
		}
		klog.V(4).Infof("Orphan Pod %s updated, objectMeta %+v -> %+v.", curPod.Name, oldPod.ObjectMeta, curPod.ObjectMeta)
		for _, set := range sets {
			ssc.enqueueStatefulSet(set)
		}
	}
}

// deletePod enqueues the statefulset for the pod accounting for deletion tombstones.
func (ssc *StatefulSetController) deletePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)

	// When a delete is dropped, the relist will notice a pod in the store not
	// in the list, leading to the insertion of a tombstone object which contains
	// the deleted key/value. Note that this value might be stale. If the pod
	// changed labels the new StatefulSet will not be woken up till the periodic resync.
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a pod %+v", obj))
			return
		}
	}

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	set := ssc.resolveControllerRef(pod.Namespace, controllerRef)
	if set == nil {
		return
	}
	klog.V(4).Infof("Pod %s/%s deleted through %v.", pod.Namespace, pod.Name, utilruntime.GetCaller())
	ssc.enqueueStatefulSet(set)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (ssc *StatefulSetController) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *appsv1alpha1.StatefulSet {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	set, err := ssc.setLister.StatefulSets(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}
	if set.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return set
}

// getStatefulSetsForPod returns a list of StatefulSets that potentially match
// a given pod.
func (ssc *StatefulSetController) getStatefulSetsForPod(pod *v1.Pod) []*appsv1alpha1.StatefulSet {
	sets, err := getPodStatefulSets(ssc.setLister, pod)
	if err != nil {
		return nil
	}
	// More than one set is selecting the same Pod
	if len(sets) > 1 {
		// ControllerRef will ensure we don't do anything crazy, but more than one
		// item in this list nevertheless constitutes user error.
		utilruntime.HandleError(
			fmt.Errorf(
				"user error: more than one StatefulSet is selecting pods with labels: %+v",
				pod.Labels))
	}
	return sets
}

func getPodStatefulSets(s kruiseappslisters.StatefulSetLister, pod *v1.Pod) ([]*appsv1alpha1.StatefulSet, error) {
	var selector labels.Selector
	var ps *appsv1alpha1.StatefulSet

	if len(pod.Labels) == 0 {
		return nil, fmt.Errorf("no StatefulSets found for pod %v because it has no labels", pod.Name)
	}

	list, err := s.StatefulSets(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var psList []*appsv1alpha1.StatefulSet
	for i := range list {
		ps = list[i]
		if ps.Namespace != pod.Namespace {
			continue
		}
		selector, err = metav1.LabelSelectorAsSelector(ps.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %v", err)
		}

		// If a StatefulSet with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		psList = append(psList, ps)
	}

	if len(psList) == 0 {
		return nil, fmt.Errorf("could not find StatefulSet for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}

	return psList, nil
}
