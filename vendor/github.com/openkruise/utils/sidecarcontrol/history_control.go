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

package sidecarcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/utils"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

var (
	patchCodec = scheme.Codecs.LegacyCodec(appsv1alpha1.SchemeGroupVersion)
)

func GetRevisionSelector(s *appsv1alpha1.SidecarSet) labels.Selector {
	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			SidecarSetKindName: s.GetName(),
		},
	}
	selector, err := utils.ValidatedLabelSelectorAsSelector(labelSelector)
	if err != nil {
		// static error, just panic
		panic("Incorrect label selector for ControllerRevision of SidecarSet.")
	}
	return selector
}

func NewRevision(s *appsv1alpha1.SidecarSet, namespace string, revision int64, collisionCount *int32) (
	*apps.ControllerRevision, error,
) {
	patch, err := getPatch(s)
	if err != nil {
		return nil, err
	}

	cr, err := NewControllerRevision(s,
		s.GetObjectKind().GroupVersionKind(),
		s.Labels,
		runtime.RawExtension{Raw: patch},
		revision,
		collisionCount)
	if err != nil {
		return nil, err
	}

	cr.SetNamespace(namespace)
	if cr.Labels == nil {
		cr.Labels = make(map[string]string)
	}
	if cr.ObjectMeta.Annotations == nil {
		cr.ObjectMeta.Annotations = make(map[string]string)
	}
	if s.Annotations[SidecarSetHashAnnotation] != "" {
		cr.Annotations[SidecarSetHashAnnotation] = s.Annotations[SidecarSetHashAnnotation]
	}
	if s.Annotations[SidecarSetHashWithoutImageAnnotation] != "" {
		cr.Annotations[SidecarSetHashWithoutImageAnnotation] = s.Annotations[SidecarSetHashWithoutImageAnnotation]
	}
	if s.Labels[appsv1alpha1.SidecarSetCustomVersionLabel] != "" {
		cr.Labels[appsv1alpha1.SidecarSetCustomVersionLabel] = s.Labels[appsv1alpha1.SidecarSetCustomVersionLabel]
	}
	cr.Labels[SidecarSetKindName] = s.Name
	for key, value := range s.Annotations {
		cr.ObjectMeta.Annotations[key] = value
	}
	return cr, nil
}

// getPatch returns a strategic merge patch that can be applied to restore a SidecarSet to a
// previous version. If the returned error is nil the patch is valid. The current state that we save is just the
// PodSpecTemplate. We can modify this later to encompass more state (or less) and remain compatible with previously
// recorded patches.
func getPatch(s *appsv1alpha1.SidecarSet) ([]byte, error) {
	str, err := runtime.Encode(patchCodec, s)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	_ = json.Unmarshal(str, &raw)

	objCopy := make(map[string]interface{})
	specCopy := make(map[string]interface{})
	// only copy some specified fields of s.Spec to specCopy
	spec := raw["spec"].(map[string]interface{})
	copySidecarSetSpecRevision(specCopy, spec)

	objCopy["spec"] = specCopy
	return json.Marshal(objCopy)
}

// NextRevision finds the next valid revision number based on revisions. If the length of revisions
// is 0 this is 1. Otherwise, it is 1 greater than the largest revision's Revision. This method
// assumes that revisions has been sorted by Revision.
func NextRevision(revisions []*apps.ControllerRevision) int64 {
	count := len(revisions)
	if count <= 0 {
		return 1
	}
	return revisions[count-1].Revision + 1
}

type HistoryControl interface {
	CreateControllerRevision(parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error)
	ListSidecarSetControllerRevisions(sidecarSet *appsv1alpha1.SidecarSet) ([]*apps.ControllerRevision, error)
	UpdateControllerRevision(revision *apps.ControllerRevision, newRevision int64) (*apps.ControllerRevision, error)
	DeleteControllerRevision(revision *apps.ControllerRevision) error
	GetHistorySidecarSet(sidecarSet *appsv1alpha1.SidecarSet, revisionInfo *appsv1alpha1.SidecarSetInjectRevision) (*appsv1alpha1.SidecarSet, error)
	GetSuitableRevisionSidecarSet(sidecarSet *appsv1alpha1.SidecarSet, oldPod, newPod *v1.Pod) (*appsv1alpha1.SidecarSet, error)
}

type realControl struct {
	// client for k8s
	client clientset.Interface
	// sidecarSet controllerRevision namespace, default is openKruise namespace
	namespace string
	// historyLister get list/get history from the shared informers's store
	historyLister appslisters.ControllerRevisionLister
}

// NewHistoryControl new history control
// indexer is controllerRevision indexer
// If you need CreateControllerRevision function, you must set parameter client
// If you need GetHistorySidecarSet and GetSuitableRevisionSidecarSet function, you must set parameter indexer
// Parameter namespace is required
func NewHistoryControl(client clientset.Interface, indexer cache.Indexer, namespace string) HistoryControl {
	return &realControl{
		client:        client,
		namespace:     namespace,
		historyLister: appslisters.NewControllerRevisionLister(indexer),
	}
}

func (r *realControl) CreateControllerRevision(parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error) {
	if collisionCount == nil {
		return nil, fmt.Errorf("collisionCount should not be nil")
	}
	// Clone the input
	clone := revision.DeepCopy()

	// Continue to attempt to create the revision updating the name with a new hash on each iteration
	for {
		hash := HashControllerRevision(revision, collisionCount)
		// Update the revisions name
		clone.Name = ControllerRevisionName(parent.GetName(), hash)
		_, err := r.client.AppsV1().ControllerRevisions(clone.Namespace).Create(context.TODO(), clone, metav1.CreateOptions{})
		if errors.IsAlreadyExists(err) {
			exists, err := r.client.AppsV1().ControllerRevisions(clone.Namespace).Get(context.TODO(), clone.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			if bytes.Equal(exists.Data.Raw, clone.Data.Raw) {
				return exists, nil
			}
			*collisionCount++
			continue
		}
		return clone, err
	}
}

func (r *realControl) ListSidecarSetControllerRevisions(sidecarSet *appsv1alpha1.SidecarSet) ([]*apps.ControllerRevision, error) {
	selector := GetRevisionSelector(sidecarSet)
	revisions, err := r.historyLister.ControllerRevisions(r.namespace).List(selector)
	if err != nil {
		klog.Errorf("Failed to get ControllerRevision for SidecarSet(%v), err: %v", sidecarSet.Name, err)
		return nil, err
	}
	return revisions, nil
}

func (r *realControl) UpdateControllerRevision(revision *apps.ControllerRevision, newRevision int64) (*apps.ControllerRevision, error) {
	clone := revision.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if clone.Revision == newRevision {
			return nil
		}
		clone.Revision = newRevision
		updated, updateErr := r.client.AppsV1().ControllerRevisions(clone.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		if updated != nil {
			clone = updated
		}
		if updated, err := r.historyLister.ControllerRevisions(clone.Namespace).Get(clone.Name); err == nil {
			// make a copy so we don't mutate the shared cache
			clone = updated.DeepCopy()
		}
		return updateErr
	})
	return clone, err
}

func (r *realControl) DeleteControllerRevision(revision *apps.ControllerRevision) error {
	return r.client.AppsV1().ControllerRevisions(r.namespace).Delete(context.TODO(), revision.Name, metav1.DeleteOptions{})
}

func (r *realControl) GetHistorySidecarSet(sidecarSet *appsv1alpha1.SidecarSet, revisionInfo *appsv1alpha1.SidecarSetInjectRevision) (*appsv1alpha1.SidecarSet, error) {
	revision, err := r.getControllerRevision(sidecarSet, revisionInfo)
	if err != nil || revision == nil {
		return nil, err
	}
	clone := sidecarSet.DeepCopy()
	cloneBytes, err := runtime.Encode(patchCodec, clone)
	if err != nil {
		klog.Errorf("Failed to encode sidecarSet(%v), error: %v", sidecarSet.Name, err)
		return nil, err
	}
	patched, err := strategicpatch.StrategicMergePatch(cloneBytes, revision.Data.Raw, clone)
	if err != nil {
		klog.Errorf("Failed to merge sidecarSet(%v) and controllerRevision(%v): %v, error: %v", sidecarSet.Name, revision.Name, err)
		return nil, err
	}
	// restore history from patch
	restoredSidecarSet := &appsv1alpha1.SidecarSet{}
	if err := json.Unmarshal(patched, restoredSidecarSet); err != nil {
		return nil, err
	}
	// restore hash annotation and revision info
	if err := restoreRevisionInfo(restoredSidecarSet, revision); err != nil {
		return nil, err
	}
	return restoredSidecarSet, nil
}

func (r *realControl) getControllerRevision(set *appsv1alpha1.SidecarSet, revisionInfo *appsv1alpha1.SidecarSetInjectRevision) (*apps.ControllerRevision, error) {
	if revisionInfo == nil {
		return nil, nil
	}
	switch {
	case revisionInfo.RevisionName != nil:
		revision, err := r.historyLister.ControllerRevisions(r.namespace).Get(*revisionInfo.RevisionName)
		if err != nil {
			klog.Errorf("Failed to get ControllerRevision %v for SidecarSet(%v), err: %v", *revisionInfo.RevisionName, set.Name, err)
			return nil, err
		}
		return revision, nil

	case revisionInfo.CustomVersion != nil:
		selector := GetRevisionSelector(set)
		req, _ := labels.NewRequirement(appsv1alpha1.SidecarSetCustomVersionLabel, selection.Equals, []string{*revisionInfo.CustomVersion})
		selector = selector.Add(*req)
		revisionList, err := r.historyLister.ControllerRevisions(r.namespace).List(selector)
		if err != nil {
			klog.Errorf("Failed to get ControllerRevision for SidecarSet(%v), custom version: %v, err: %v", set.Name, *revisionInfo.CustomVersion, err)
			return nil, err
		}

		var revisions []*apps.ControllerRevision
		for i := range revisionList {
			obj := revisionList[i]
			revisions = append(revisions, obj)
		}

		if len(revisions) == 0 {
			return nil, generateNotFoundError(set)
		}
		SortControllerRevisions(revisions)
		return revisions[len(revisions)-1], nil
	}

	klog.Error("Failed to get controllerRevision due to both empty RevisionName and CustomVersion")
	return nil, nil
}

func copySidecarSetSpecRevision(dst, src map[string]interface{}) {
	// we will use patch instead of update operation to update pods in the future
	// dst["$patch"] = "replace"
	// only record these revisions
	dst["volumes"] = src["volumes"]
	dst["containers"] = src["containers"]
	dst["initContainers"] = src["initContainers"]
	dst["imagePullSecrets"] = src["imagePullSecrets"]
	dst["patchPodMetadata"] = src["patchPodMetadata"]
}

func restoreRevisionInfo(sidecarSet *appsv1alpha1.SidecarSet, revision *apps.ControllerRevision) error {
	if sidecarSet.Annotations == nil {
		sidecarSet.Annotations = map[string]string{}
	}
	if revision.Annotations[SidecarSetHashAnnotation] != "" {
		sidecarSet.Annotations[SidecarSetHashAnnotation] = revision.Annotations[SidecarSetHashAnnotation]
	} else {
		hashCodeWithImage, err := SidecarSetHash(sidecarSet)
		if err != nil {
			return err
		}
		sidecarSet.Annotations[SidecarSetHashAnnotation] = hashCodeWithImage
	}
	if revision.Annotations[SidecarSetHashWithoutImageAnnotation] != "" {
		sidecarSet.Annotations[SidecarSetHashWithoutImageAnnotation] = revision.Annotations[SidecarSetHashWithoutImageAnnotation]
	} else {
		hashCodeWithoutImage, err := SidecarSetHashWithoutImage(sidecarSet)
		if err != nil {
			return err
		}
		sidecarSet.Annotations[SidecarSetHashWithoutImageAnnotation] = hashCodeWithoutImage
	}
	sidecarSet.Status.LatestRevision = revision.Name
	return nil
}

func MockSidecarSetForRevision(set *appsv1alpha1.SidecarSet) metav1.Object {
	return &metav1.ObjectMeta{
		UID:       set.UID,
		Name:      set.Name,
		Namespace: utils.GetNamespace(),
	}
}

func generateNotFoundError(set *appsv1alpha1.SidecarSet) error {
	return errors.NewNotFound(schema.GroupResource{
		Group:    apps.GroupName,
		Resource: "ControllerRevision",
	}, set.Name)
}

func (r *realControl) GetSuitableRevisionSidecarSet(sidecarSet *appsv1alpha1.SidecarSet, oldPod, newPod *v1.Pod) (*appsv1alpha1.SidecarSet, error) {
	// operation == update
	if oldPod != nil {
		// optimization: quickly return if newPod matched the latest sidecarSet
		if GetPodSidecarSetRevision(sidecarSet.Name, newPod) == GetSidecarSetRevision(sidecarSet) {
			return sidecarSet.DeepCopy(), nil
		}
		selector := GetRevisionSelector(sidecarSet)
		revisions, err := r.historyLister.ControllerRevisions(r.namespace).List(selector)
		if err != nil {
			klog.Errorf("Failed to get ControllerRevision for SidecarSet(%v), err: %v", sidecarSet.Name, err)
			return nil, err
		}

		suitableSidecarSet, err := r.getSpecificRevisionSidecarSetForPod(sidecarSet, revisions, newPod)
		if err != nil {
			return nil, err
		} else if suitableSidecarSet != nil {
			return suitableSidecarSet, nil
		}

		suitableSidecarSet, err = r.getSpecificRevisionSidecarSetForPod(sidecarSet, revisions, oldPod)
		if err != nil {
			return nil, err
		} else if suitableSidecarSet != nil {
			return suitableSidecarSet, nil
		}

		return sidecarSet.DeepCopy(), nil
	}

	revisionInfo := sidecarSet.Spec.InjectionStrategy.Revision
	if revisionInfo == nil || (revisionInfo.RevisionName == nil && revisionInfo.CustomVersion == nil) {
		return sidecarSet.DeepCopy(), nil
	}

	// TODO: support 'PartitionBased' policy to inject old/new revision according to Partition
	switch sidecarSet.Spec.InjectionStrategy.Revision.Policy {
	case "", appsv1alpha1.AlwaysSidecarSetInjectRevisionPolicy:
		return r.getSpecificHistorySidecarSet(sidecarSet, revisionInfo)
	}

	return r.getSpecificHistorySidecarSet(sidecarSet, revisionInfo)
}

func (r *realControl) getSpecificRevisionSidecarSetForPod(sidecarSet *appsv1alpha1.SidecarSet, revisions []*apps.ControllerRevision, pod *corev1.Pod) (*appsv1alpha1.SidecarSet, error) {
	var err error
	var matchedSidecarSet *appsv1alpha1.SidecarSet
	for _, revision := range revisions {
		if GetPodSidecarSetControllerRevision(sidecarSet.Name, pod) == revision.Name {
			matchedSidecarSet, err = r.getSpecificHistorySidecarSet(sidecarSet, &appsv1alpha1.SidecarSetInjectRevision{RevisionName: &revision.Name})
			if err != nil {
				return nil, err
			}
			break
		}
	}
	return matchedSidecarSet, nil
}

func (r *realControl) getSpecificHistorySidecarSet(sidecarSet *appsv1alpha1.SidecarSet, revisionInfo *appsv1alpha1.SidecarSetInjectRevision) (*appsv1alpha1.SidecarSet, error) {
	// else return its corresponding history revision
	historySidecarSet, err := r.GetHistorySidecarSet(sidecarSet, revisionInfo)
	if err != nil {
		klog.Warningf("Failed to restore history revision for SidecarSet %v, ControllerRevision name %v:, error: %v",
			sidecarSet.Name, sidecarSet.Spec.InjectionStrategy.Revision, err)
		return nil, err
	}
	if historySidecarSet == nil {
		historySidecarSet = sidecarSet.DeepCopy()
		klog.Warningf("Failed to restore history revision for SidecarSet %v, will use the latest", sidecarSet.Name)
	}
	return historySidecarSet, nil
}

// ControllerRevisionHashLabel is the label used to indicate the hash value of a ControllerRevision's Data.
const ControllerRevisionHashLabel = "controller.kubernetes.io/hash"

// ControllerRevisionName returns the Name for a ControllerRevision in the form prefix-hash. If the length
// of prefix is greater than 223 bytes, it is truncated to allow for a name that is no larger than 253 bytes.
func ControllerRevisionName(prefix string, hash string) string {
	if len(prefix) > 223 {
		prefix = prefix[:223]
	}

	return fmt.Sprintf("%s-%s", prefix, hash)
}

// NewControllerRevision returns a ControllerRevision with a ControllerRef pointing to parent and indicating that
// parent is of parentKind. The ControllerRevision has labels matching template labels, contains Data equal to data, and
// has a Revision equal to revision. The collisionCount is used when creating the name of the ControllerRevision
// so the name is likely unique. If the returned error is nil, the returned ControllerRevision is valid. If the
// returned error is not nil, the returned ControllerRevision is invalid for use.
func NewControllerRevision(parent metav1.Object,
	parentKind schema.GroupVersionKind,
	templateLabels map[string]string,
	data runtime.RawExtension,
	revision int64,
	collisionCount *int32) (*apps.ControllerRevision, error) {
	labelMap := make(map[string]string)
	for k, v := range templateLabels {
		labelMap[k] = v
	}
	cr := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labelMap,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(parent, parentKind)},
		},
		Data:     data,
		Revision: revision,
	}
	hash := HashControllerRevision(cr, collisionCount)
	cr.Name = ControllerRevisionName(parent.GetName(), hash)
	cr.Labels[ControllerRevisionHashLabel] = hash
	return cr, nil
}

// HashControllerRevision hashes the contents of revision's Data using FNV hashing. If probe is not nil, the byte value
// of probe is added written to the hash as well. The returned hash will be a safe encoded string to avoid bad words.
func HashControllerRevision(revision *apps.ControllerRevision, probe *int32) string {
	hf := fnv.New32()
	if len(revision.Data.Raw) > 0 {
		hf.Write(revision.Data.Raw)
	}
	if revision.Data.Object != nil {
		DeepHashObject(hf, revision.Data.Object)
	}
	if probe != nil {
		hf.Write([]byte(strconv.FormatInt(int64(*probe), 10)))
	}
	return rand.SafeEncodeString(fmt.Sprint(hf.Sum32()))
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hasher, "%#v", objectToWrite)
}

// SortControllerRevisions sorts revisions by their Revision.
func SortControllerRevisions(revisions []*apps.ControllerRevision) {
	sort.Stable(byRevision(revisions))
}

// byRevision implements sort.Interface to allow ControllerRevisions to be sorted by Revision.
type byRevision []*apps.ControllerRevision

func (br byRevision) Len() int {
	return len(br)
}

// Less breaks ties first by creation timestamp, then by name
func (br byRevision) Less(i, j int) bool {
	if br[i].Revision == br[j].Revision {
		if br[j].CreationTimestamp.Equal(&br[i].CreationTimestamp) {
			return br[i].Name < br[j].Name
		}
		return br[j].CreationTimestamp.After(br[i].CreationTimestamp.Time)
	}
	return br[i].Revision < br[j].Revision
}

func (br byRevision) Swap(i, j int) {
	br[i], br[j] = br[j], br[i]
}
