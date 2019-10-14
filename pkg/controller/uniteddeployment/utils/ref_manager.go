package utils

import (
	"fmt"
	"reflect"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// GetOwner is the type of function which is provided to get the owner resource.
type GetOwner func() (runtime.Object, error)

// UpdateOwnee is the type of function which is provided to update a resource after ownerReference is updated.
type UpdateOwnee func(runtime.Object) error

// RefManager provides the method to
type RefManager struct {
	getOwner    GetOwner
	updateOwnee UpdateOwnee
	selector    labels.Selector
	owner       metav1.Object
	ownerType   reflect.Type
	schema      *runtime.Scheme

	once        sync.Once
	canAdoptErr error
}

// NewRefManager returns a RefManager that exposes
// methods to manage the controllerRef of pods.
func NewRefManager(getOwner GetOwner, updateOwnee UpdateOwnee, selector *metav1.LabelSelector, owner metav1.Object, schema *runtime.Scheme) (*RefManager, error) {
	s, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}

	ownerType := reflect.TypeOf(owner)
	if ownerType.Kind() == reflect.Ptr {
		ownerType = ownerType.Elem()
	}
	return &RefManager{
		getOwner:    getOwner,
		updateOwnee: updateOwnee,
		selector:    s,
		owner:       owner,
		ownerType:   ownerType,
		schema:      schema,
	}, nil
}

// ClaimOwnedObjects tries to take ownership of a list of objects for this controller.
func (mgr *RefManager) ClaimOwnedObjects(objs []metav1.Object, filters ...func(metav1.Object) bool) ([]metav1.Object, error) {
	match := func(obj metav1.Object) bool {
		if !mgr.selector.Matches(labels.Set(obj.GetLabels())) {
			return false
		}

		for _, filter := range filters {
			if !filter(obj) {
				return false
			}
		}

		return true
	}

	claimObjs := []metav1.Object{}
	errlist := []error{}
	for _, obj := range objs {
		ok, err := mgr.claimObject(obj, match)
		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimObjs = append(claimObjs, obj)
		}
	}
	return claimObjs, utilerrors.NewAggregate(errlist)
}

func (mgr *RefManager) canAdoptOnce() error {
	mgr.once.Do(func() {
		mgr.canAdoptErr = mgr.canAdopt()
	})

	return mgr.canAdoptErr
}

func (mgr *RefManager) canAdopt() error {
	fresh, err := mgr.getOwner()
	if err != nil {
		return err
	}

	freshObj, ok := fresh.(metav1.Object)
	if !ok {
		return fmt.Errorf("expected k8s.io/apimachinery/pkg/apis/meta/v1.object when getting owner %v/%v UID %v",
			mgr.owner.GetNamespace(), mgr.owner.GetName(), mgr.owner.GetUID())
	}

	if freshObj.GetUID() != mgr.owner.GetUID() {
		return fmt.Errorf("original owner %v/%v is gone: got uid %v, wanted %v",
			mgr.owner.GetNamespace(), mgr.owner.GetName(), freshObj.GetUID(), mgr.owner.GetUID())
	}

	if freshObj.GetDeletionTimestamp() != nil {
		return fmt.Errorf("%v/%v has just been deleted at %v",
			mgr.owner.GetNamespace(), mgr.owner.GetName(), freshObj.GetDeletionTimestamp())
	}

	return nil
}

func (mgr *RefManager) adopt(obj metav1.Object) error {
	if err := mgr.canAdoptOnce(); err != nil {
		return fmt.Errorf("can't adopt Object %v/%v (%v): %v", obj.GetNamespace(), obj.GetName(), obj.GetUID(), err)
	}

	if mgr.schema == nil {
		return nil
	}

	if err := controllerutil.SetControllerReference(mgr.owner, obj, mgr.schema); err != nil {
		return fmt.Errorf("can't set Object %v/%v (%v) owner reference: %v", obj.GetNamespace(), obj.GetName(), obj.GetUID(), err)
	}

	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		return fmt.Errorf("can't update Object %v/%v (%v) owner reference: fail to cast to runtime.Object", obj.GetNamespace(), obj.GetName(), obj.GetUID())
	}

	if err := mgr.updateOwnee(runtimeObj); err != nil {
		return fmt.Errorf("can't update Object %v/%v (%v) owner reference: %v", obj.GetNamespace(), obj.GetName(), obj.GetUID(), err)
	}
	return nil
}

func (mgr *RefManager) release(obj metav1.Object) error {
	idx := -1
	for i, ref := range obj.GetOwnerReferences() {
		if ref.UID == mgr.owner.GetUID() {
			idx = i
			break
		}
	}
	if idx > -1 {
		runtimeObj, ok := obj.(runtime.Object)
		if !ok {
			return fmt.Errorf("can't remove Pod %v/%v (%v) owner reference: fail to cast to runtime.Object", obj.GetNamespace(), obj.GetName(), obj.GetUID())
		}

		obj.SetOwnerReferences(append(obj.GetOwnerReferences()[:idx], obj.GetOwnerReferences()[idx+1:]...))
		if err := mgr.updateOwnee(runtimeObj); err != nil {
			return fmt.Errorf("can't remove Pod %v/%v (%v) owner reference %v/%v (%v): %v",
				obj.GetNamespace(), obj.GetName(), obj.GetUID(), obj.GetNamespace(), obj.GetName(), mgr.owner.GetUID(), err)
		}
	}

	return nil
}

func (mgr *RefManager) claimObject(obj metav1.Object, match func(metav1.Object) bool) (bool, error) {
	controllerRef := metav1.GetControllerOf(obj)
	if controllerRef != nil {
		if controllerRef.UID != mgr.owner.GetUID() {
			// Owned by someone else. Ignore.
			return false, nil
		}
		if match(obj) {
			// We already own it and the selector matches.
			// Return true (successfully claimed) before checking deletion timestamp.
			// We're still allowed to claim things we already own while being deleted
			// because doing so requires taking no actions.
			return true, nil
		}
		// Owned by us but selector doesn't match.
		// Try to release, unless we're being deleted.
		if mgr.owner.GetDeletionTimestamp() != nil {
			return false, nil
		}
		if err := mgr.release(obj); err != nil {
			// If the pod no longer exists, ignore the error.
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			// Either someone else released it, or there was a transient error.
			// The controller should requeue and try again if it's still stale.
			return false, err
		}
		// Successfully released.
		return false, nil
	}

	// It's an orphan.
	if mgr.owner.GetDeletionTimestamp() != nil || !match(obj) {
		// Ignore if we're being deleted or selector doesn't match.
		return false, nil
	}
	if obj.GetDeletionTimestamp() != nil {
		// Ignore if the object is being deleted
		return false, nil
	}
	// Selector matches. Try to adopt.
	if err := mgr.adopt(obj); err != nil {
		// If the pod no longer exists, ignore the error.
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		// Either someone else claimed it first, or there was a transient error.
		// The controller should requeue and try again if it's still orphaned.
		return false, err
	}
	// Successfully adopted.
	return true, nil
}
