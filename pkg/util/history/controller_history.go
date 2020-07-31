/*
Copyright 2019 The Kruise Authors.

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

package history

import (
	"bytes"
	"context"
	"fmt"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewHistory returns an an instance of Interface that uses client to communicate with the API Server and lister to list ControllerRevisions.
func NewHistory(c client.Client) history.Interface {
	return &realHistory{Client: c}
}

type realHistory struct {
	client.Client
}

func (rh *realHistory) ListControllerRevisions(parent metav1.Object, selector labels.Selector) ([]*apps.ControllerRevision, error) {
	// List all revisions in the namespace that match the selector
	revisions := apps.ControllerRevisionList{}
	err := rh.List(context.TODO(), &revisions, &client.ListOptions{Namespace: parent.GetNamespace(), LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var owned []*apps.ControllerRevision
	for i := range revisions.Items {
		ref := metav1.GetControllerOf(&revisions.Items[i])
		if ref == nil || ref.UID == parent.GetUID() {
			owned = append(owned, &revisions.Items[i])
		}
	}
	return owned, err
}

func (rh *realHistory) CreateControllerRevision(parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error) {
	if collisionCount == nil {
		return nil, fmt.Errorf("collisionCount should not be nil")
	}
	ns := parent.GetNamespace()

	// Clone the input
	clone := revision.DeepCopy()

	// Continue to attempt to create the revision updating the name with a new hash on each iteration
	for {
		hash := history.HashControllerRevision(revision, collisionCount)
		// Update the revisions name
		clone.Name = history.ControllerRevisionName(parent.GetName(), hash)

		created := clone.DeepCopy()
		created.Namespace = ns
		err := rh.Create(context.TODO(), created)
		if errors.IsAlreadyExists(err) {
			exists := apps.ControllerRevision{}
			err = rh.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: clone.Name}, &exists)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(exists.Data.Raw, clone.Data.Raw) {
				return &exists, nil
			}
			*collisionCount++
			continue
		}
		return created, err
	}
}

func (rh *realHistory) UpdateControllerRevision(revision *apps.ControllerRevision, newRevision int64) (*apps.ControllerRevision, error) {
	clone := revision.DeepCopy()
	namespacedName := types.NamespacedName{Namespace: clone.Namespace, Name: clone.Name}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if clone.Revision == newRevision {
			return nil
		}
		clone.Revision = newRevision
		updateErr := rh.Update(context.TODO(), clone)
		if updateErr == nil {
			return nil
		}
		got := &apps.ControllerRevision{}
		if err := rh.Get(context.TODO(), namespacedName, got); err == nil {
			clone = got
		}
		return updateErr
	})
	return clone, err
}

func (rh *realHistory) DeleteControllerRevision(revision *apps.ControllerRevision) error {
	return rh.Delete(context.TODO(), revision)
}

func (rh *realHistory) AdoptControllerRevision(parent metav1.Object, parentKind schema.GroupVersionKind, revision *apps.ControllerRevision) (*apps.ControllerRevision, error) {
	clone := revision.DeepCopy()
	namespacedName := types.NamespacedName{Namespace: clone.Namespace, Name: clone.Name}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Return an error if the parent does not own the revision
		if owner := metav1.GetControllerOf(clone); owner != nil {
			return fmt.Errorf("attempt to adopt revision owned by %v", owner)
		}

		valTrue := true
		clone.OwnerReferences = append(clone.OwnerReferences, metav1.OwnerReference{
			APIVersion:         parentKind.GroupVersion().String(),
			Kind:               parentKind.Kind,
			Name:               parent.GetName(),
			UID:                parent.GetUID(),
			Controller:         &valTrue,
			BlockOwnerDeletion: &valTrue,
		})

		updateErr := rh.Update(context.TODO(), clone)
		if updateErr == nil {
			return nil
		}

		got := &apps.ControllerRevision{}
		if err := rh.Get(context.TODO(), namespacedName, got); err == nil {
			clone = got
		}
		return updateErr
	})

	if err != nil {
		return nil, err
	}
	return clone, nil
}

func (rh *realHistory) ReleaseControllerRevision(parent metav1.Object, revision *apps.ControllerRevision) (*apps.ControllerRevision, error) {
	clone := revision.DeepCopy()
	namespacedName := types.NamespacedName{Namespace: clone.Namespace, Name: clone.Name}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Return an error if the parent does not own the revision
		if owner := metav1.GetControllerOf(clone); owner != nil {
			return fmt.Errorf("attempt to adopt revision owned by %v", owner)
		}

		var newOwners []metav1.OwnerReference
		for _, o := range clone.OwnerReferences {
			if o.UID == parent.GetUID() {
				continue
			}
			newOwners = append(newOwners, o)
		}
		clone.OwnerReferences = newOwners
		updateErr := rh.Update(context.TODO(), clone)
		if updateErr == nil {
			return nil
		}

		got := &apps.ControllerRevision{}
		if err := rh.Get(context.TODO(), namespacedName, got); err == nil {
			clone = got
		}
		return updateErr
	})

	if err != nil {
		if errors.IsNotFound(err) {
			// We ignore deleted revisions
			return nil, nil
		}
		if errors.IsInvalid(err) {
			// We ignore cases where the parent no longer owns the revision or where the revision has no
			// owner.
			return nil, nil
		}
		return nil, err
	}
	return clone, nil
}
