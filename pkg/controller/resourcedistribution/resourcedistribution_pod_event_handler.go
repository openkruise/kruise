package resourcedistribution

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForNamespace{}

type enqueueRequestForNamespace struct {
	reader client.Reader
}

func (p *enqueueRequestForNamespace) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addNamespace(q, evt.Object)
}

func (p *enqueueRequestForNamespace) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForNamespace) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForNamespace) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updateNamespace(q, evt.ObjectOld, evt.ObjectNew)
}

// When a Namespace is created, figure out what ResourceDistribution it will work on it and enqueue them.
// obj must have *v1.Namespace type.
func (p *enqueueRequestForNamespace) addNamespace(q workqueue.RateLimitingInterface, obj runtime.Object) {
	namespace, ok := obj.(*corev1.Namespace)
	if !ok {
		return
	}

	resourceDistributions, err := p.getNamespaceMatchedResourceDistributions(namespace)
	if err != nil {
		klog.Errorf("unable to get ResourceDistribution related with namespaces %s, err: %v", namespace.Name, err)
		return
	}

	for _, resourceDistribution := range resourceDistributions {
		klog.V(3).Infof("create Namespace(%s) and reconcile ResourceDistribution(%s)", namespace.Name, resourceDistribution.Name)
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: resourceDistribution.Name,
			},
		})
	}
}

// When labels of a Namespace are created, figure out what ResourceDistribution it will work on it and enqueue them.
// objOld and objNew must have *v1.Namespace type.
func (p *enqueueRequestForNamespace) updateNamespace(q workqueue.RateLimitingInterface, objOld, objNew runtime.Object) {
	namespaceOld, okOld := objOld.(*corev1.Namespace)
	namespaceNew, okNew := objNew.(*corev1.Namespace)
	if !okOld || !okNew || reflect.DeepEqual(namespaceNew.ObjectMeta.Labels, namespaceOld.ObjectMeta.Labels) {
		return
	}

	resourceDistributionMatchedOld, errOld := p.getNamespaceMatchedResourceDistributions(namespaceOld)
	resourceDistributionMatchedNew, errNew := p.getNamespaceMatchedResourceDistributions(namespaceNew)
	resourceDistributions := append(resourceDistributionMatchedOld, resourceDistributionMatchedNew...)
	if errOld != nil || errNew != nil {
		klog.Errorf("unable to get ResourceDistribution related with namespaces %s, errs: [(old)%v, (new)%v]", namespaceNew.Name, errOld, errNew)
		return
	}

	for _, resourceDistribution := range resourceDistributions {
		klog.V(3).Infof("create Namespace(%s) and reconcile ResourceDistribution(%s)", namespaceNew.Name, resourceDistribution.Name)
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: resourceDistribution.Name,
			},
		})
	}
}

// getNamespaceMatchedResourceDistributions returns all matched ResourceDistributions via labelSelector
func (p *enqueueRequestForNamespace) getNamespaceMatchedResourceDistributions(namespace *corev1.Namespace) ([]*appsv1alpha1.ResourceDistribution, error) {
	var matchedResourceDistributions []*appsv1alpha1.ResourceDistribution

	// if ResourceDistribution has been injected into namespace.Annotations
	resourceDistributionNames, ok := namespace.Annotations[ResourceDistributionListAnnotation]
	if ok && len(resourceDistributionNames) > 0 {
		for _, resourceDistributionName := range strings.Split(resourceDistributionNames, ",") {
			ResourceDistribution := new(appsv1alpha1.ResourceDistribution)
			if err := p.reader.Get(context.TODO(), types.NamespacedName{
				Name: resourceDistributionName,
			}, ResourceDistribution); err != nil {
				if errors.IsNotFound(err) {
					klog.Errorf("ResourceDistribution %s not found", resourceDistributionName)
					continue
				}
				return nil, err
			}
			matchedResourceDistributions = append(matchedResourceDistributions, ResourceDistribution)
		}
		return matchedResourceDistributions, nil
	}

	// else check all ResourceDistributions
	ResourceDistributions := &appsv1alpha1.ResourceDistributionList{}
	if err := p.reader.List(context.TODO(), ResourceDistributions); err != nil {
		return nil, err
	}

	for _, resourceDistribution := range ResourceDistributions.Items {
		fmt.Printf("WTF: %s : %v\n\n", resourceDistribution.Name, resourceDistribution.Spec.Targets.NamespaceLabelSelector.MatchLabels)
		matched, err := NamespaceMatchedResourceDistribution(namespace, &resourceDistribution)
		if err != nil {
			return nil, err
		}
		if matched {
			matchedResourceDistributions = append(matchedResourceDistributions, &resourceDistribution)
		}
	}
	return matchedResourceDistributions, nil
}
