/*
Copyright 2022 The Kruise Authors.
Copyright 2018 The Kubernetes Authors.

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

package client

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type noDeepCopyLister struct {
	cache  cache.Cache
	scheme *runtime.Scheme
}

func (r *noDeepCopyLister) List(ctx context.Context, out client.ObjectList, opts ...client.ListOption) error {
	startTime := time.Now()
	gvk, _, err := r.objectTypeForListObject(out)
	if err != nil {
		return err
	}
	indexer, err := r.getIndexerByGVK(ctx, *gvk)
	if err != nil {
		return err
	}

	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	var objs []interface{}
	switch {
	case listOpts.FieldSelector != nil:
		field, val, requiresExact := requiresExactMatch(listOpts.FieldSelector)
		if !requiresExact {
			return fmt.Errorf("non-exact field matches are not supported by the cache")
		}
		// list all objects by the field selector.  If this is namespaced and we have one, ask for the
		// namespaced index key.  Otherwise, ask for the non-namespaced variant by using the fake "all namespaces"
		// namespace.
		objs, err = indexer.ByIndex(FieldIndexName(field), KeyToNamespacedKey(listOpts.Namespace, val))
	case listOpts.Namespace != "":
		objs, err = indexer.ByIndex(toolscache.NamespaceIndex, listOpts.Namespace)
	default:
		objs = indexer.List()
	}
	if err != nil {
		return err
	}

	var labelSel labels.Selector
	if listOpts.LabelSelector != nil {
		labelSel = listOpts.LabelSelector
	}
	limitSet := listOpts.Limit > 0

	runtimeObjs := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		// if the Limit option is set and the number of items
		// listed exceeds this limit, then stop reading.
		if limitSet && int64(len(runtimeObjs)) >= listOpts.Limit {
			break
		}
		obj, isObj := item.(runtime.Object)
		if !isObj {
			return fmt.Errorf("cache contained %T, which is not an Object", obj)
		}
		meta, err := apimeta.Accessor(obj)
		if err != nil {
			return err
		}
		if labelSel != nil {
			lbls := labels.Set(meta.GetLabels())
			if !labelSel.Matches(lbls) {
				continue
			}
		}
		runtimeObjs = append(runtimeObjs, obj)
	}
	defer func() {
		klog.V(6).Infof("Listed %v %v objects %v without DeepCopy, cost %v", gvk.GroupVersion(), gvk.Kind, len(runtimeObjs), time.Since(startTime))
	}()
	return apimeta.SetList(out, runtimeObjs)
}

func (r *noDeepCopyLister) getIndexerByGVK(ctx context.Context, gvk schema.GroupVersionKind) (toolscache.Indexer, error) {
	informer, err := r.cache.GetInformerForKind(ctx, gvk)
	if err != nil {
		return nil, err
	}
	sharedInformer, ok := informer.(toolscache.SharedIndexInformer)
	if !ok {
		return nil, fmt.Errorf("informer %T from cache is not a SharedIndexInformer", informer)
	}
	return sharedInformer.GetIndexer(), nil
}

// objectTypeForListObject tries to find the runtime.Object and associated GVK
// for a single object corresponding to the passed-in list type. We need them
// because they are used as cache map key.
func (r *noDeepCopyLister) objectTypeForListObject(list client.ObjectList) (*schema.GroupVersionKind, runtime.Object, error) {
	gvk, err := apiutil.GVKForObject(list, r.scheme)
	if err != nil {
		return nil, nil, err
	}

	if !strings.HasSuffix(gvk.Kind, "List") {
		return nil, nil, fmt.Errorf("non-list type %T (kind %q) passed as output", list, gvk)
	}
	// we need the non-list GVK, so chop off the "List" from the end of the kind
	gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	_, isUnstructured := list.(*unstructured.UnstructuredList)
	var cacheTypeObj runtime.Object
	if isUnstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		cacheTypeObj = u
	} else {
		itemsPtr, err := apimeta.GetItemsPtr(list)
		if err != nil {
			return nil, nil, err
		}
		// http://knowyourmeme.com/memes/this-is-fine
		elemType := reflect.Indirect(reflect.ValueOf(itemsPtr)).Type().Elem()
		if elemType.Kind() != reflect.Ptr {
			elemType = reflect.PtrTo(elemType)
		}

		cacheTypeValue := reflect.Zero(elemType)
		var ok bool
		cacheTypeObj, ok = cacheTypeValue.Interface().(runtime.Object)
		if !ok {
			return nil, nil, fmt.Errorf("cannot get cache for %T, its element %T is not a runtime.Object", list, cacheTypeValue.Interface())
		}
	}

	return &gvk, cacheTypeObj, nil
}

// requiresExactMatch checks if the given field selector is of the form `k=v` or `k==v`.
func requiresExactMatch(sel fields.Selector) (field, val string, required bool) {
	reqs := sel.Requirements()
	if len(reqs) != 1 {
		return "", "", false
	}
	req := reqs[0]
	if req.Operator != selection.Equals && req.Operator != selection.DoubleEquals {
		return "", "", false
	}
	return req.Field, req.Value, true
}

// FieldIndexName constructs the name of the index over the given field,
// for use with an indexer.
func FieldIndexName(field string) string {
	return "field:" + field
}

// noNamespaceNamespace is used as the "namespace" when we want to list across all namespaces.
const allNamespacesNamespace = "__all_namespaces"

// KeyToNamespacedKey prefixes the given index key with a namespace
// for use in field selector indexes.
func KeyToNamespacedKey(ns string, baseKey string) string {
	if ns != "" {
		return ns + "/" + baseKey
	}
	return allNamespacesNamespace + "/" + baseKey
}
