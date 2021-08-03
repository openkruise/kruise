package resourcedistribution

import (
	"context"
	"testing"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var (
	enqueueHandler = &enqueueRequestForNamespace{}
)

func init() {

}

func TestNamespaceMatchedResourceDistribution(t *testing.T) {
	distributor := buildResourceDistribution(runtime.RawExtension{})
	env := append(makeEnvironment(), distributor)
	handlerClient := fake.NewFakeClientWithScheme(scheme, env...)
	enqueueHandler.reader = handlerClient

	fetched := &appsalphav1.ResourceDistribution{}
	if err := handlerClient.Get(context.TODO(), types.NamespacedName{Name: distributor.Name}, fetched); err != nil {
		t.Fatalf("make env failed, err %v", err)
	}

	// case 1
	namespaceDemo1 := &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: "ns-6",
			Labels: map[string]string{
				"group": "one",
			},
		},
	}
	testEnqueueRequestForNamespaceCreate(namespaceDemo1, 1, t)

	// case 2
	namespaceDemo2 := &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: "ns-6",
			Annotations: map[string]string{
				ResourceDistributionListAnnotation: "test-resource-distribution,test-rd",
			},
			Labels: map[string]string{
				"group": "one",
			},
		},
	}
	testEnqueueRequestForNamespaceCreate(namespaceDemo2, 1, t)

	// case 3
	namespaceDemo3 := namespaceDemo2.DeepCopy()
	namespaceDemo3.Annotations = nil
	namespaceDemo3.ObjectMeta.Labels["group"] = "seven"
	distributor2 := distributor.DeepCopy()
	distributor2.Name = "test-resource-distribution-2"
	distributor2.Spec.Targets.NamespaceLabelSelector.MatchLabels["group"] = "seven"
	if err := handlerClient.Create(context.TODO(), distributor2, &client.CreateOptions{}); err != nil {
		t.Fatalf("add distributor2 to fake client, err %v", err)
	}
	testEnqueueRequestForNamespaceUpdate(namespaceDemo2, namespaceDemo3, 2, t)
}

func testEnqueueRequestForNamespaceCreate(namespaceDemo *corev1.Namespace, expectedNumber int, t *testing.T) {
	// create
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createEvt := event.CreateEvent{
		Object: namespaceDemo,
	}
	enqueueHandler.Create(createEvt, createQ)
	if createQ.Len() != expectedNumber {
		t.Errorf("unexpected create event handle queue size, expected %d actual %d", expectedNumber, createQ.Len())
	}
}

func testEnqueueRequestForNamespaceUpdate(namespaceOld, namespaceNew *corev1.Namespace, expectedNumber int, t *testing.T) {
	// update
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createEvt := event.UpdateEvent{
		ObjectOld: namespaceOld,
		ObjectNew: namespaceNew,
	}
	enqueueHandler.Update(createEvt, createQ)
	if createQ.Len() != expectedNumber {
		t.Errorf("unexpected update event handle queue size, expected %d actual %d", expectedNumber, createQ.Len())
	}
}
