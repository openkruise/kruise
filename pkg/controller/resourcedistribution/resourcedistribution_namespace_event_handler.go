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

package resourcedistribution

import (
	"context"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

var _ handler.TypedEventHandler[*corev1.Namespace] = &enqueueRequestForNamespace{}

type matchFunc func(*corev1.Namespace, *appsv1alpha1.ResourceDistribution) (bool, error)

type enqueueRequestForNamespace struct {
	reader client.Reader
}

func (p *enqueueRequestForNamespace) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Namespace], q workqueue.RateLimitingInterface) {
	p.addNamespace(q, evt.Object, matchViaTargets)
}
func (p *enqueueRequestForNamespace) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Namespace], q workqueue.RateLimitingInterface) {
	p.addNamespace(q, evt.Object, matchViaIncludedNamespaces)
}
func (p *enqueueRequestForNamespace) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Namespace], q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForNamespace) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Namespace], q workqueue.RateLimitingInterface) {
	p.updateNamespace(q, evt.ObjectOld, evt.ObjectNew)
}

// When a Namespace was created or deleted, figure out what ResourceDistribution it will work on it and enqueue them.
// obj must have *v1.Namespace type.
func (p *enqueueRequestForNamespace) addNamespace(q workqueue.RateLimitingInterface, obj runtime.Object, fn matchFunc) {
	namespace, ok := obj.(*corev1.Namespace)
	if !ok {
		return
	}

	resourceDistributions, err := p.getNamespaceMatchedResourceDistributions(namespace, fn)
	if err != nil {
		klog.ErrorS(err, "Unable to get the ResourceDistributions related with namespace", "namespace", namespace.Name)
		return
	}
	addMatchedResourceDistributionToWorkQueue(q, resourceDistributions)
}

// When labels of a Namespace were updated, figure out what ResourceDistribution it will work on it and enqueue them.
// objOld and objNew must have *v1.Namespace type.
func (p *enqueueRequestForNamespace) updateNamespace(q workqueue.RateLimitingInterface, objOld, objNew runtime.Object) {
	namespaceOld, okOld := objOld.(*corev1.Namespace)
	namespaceNew, okNew := objNew.(*corev1.Namespace)
	if !okOld || !okNew || reflect.DeepEqual(namespaceNew.ObjectMeta.Labels, namespaceOld.ObjectMeta.Labels) {
		return
	}
	p.addNamespace(q, objNew, matchViaLabelSelector)
	p.addNamespace(q, objOld, matchViaLabelSelector)
}

// getNamespaceMatchedResourceDistributions returns all matched ResourceDistributions via labelSelector
func (p *enqueueRequestForNamespace) getNamespaceMatchedResourceDistributions(namespace *corev1.Namespace, match matchFunc) ([]*appsv1alpha1.ResourceDistribution, error) {
	var matchedResourceDistributions []*appsv1alpha1.ResourceDistribution
	ResourceDistributions := &appsv1alpha1.ResourceDistributionList{}
	if err := p.reader.List(context.TODO(), ResourceDistributions); err != nil {
		return nil, err
	}

	for i := range ResourceDistributions.Items {
		resourceDistribution := &ResourceDistributions.Items[i]
		matched, err := match(namespace, resourceDistribution)
		if err != nil {
			return nil, err
		}
		if matched {
			matchedResourceDistributions = append(matchedResourceDistributions, resourceDistribution)
		}
	}
	return matchedResourceDistributions, nil
}
