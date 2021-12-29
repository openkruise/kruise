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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	patchCodec = scheme.Codecs.LegacyCodec(appsv1alpha1.SchemeGroupVersion)
)

type HistoryControl interface {
	CreateControllerRevision(parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error)
	NewRevision(s *appsv1alpha1.SidecarSet, namespace string, revision int64, collisionCount *int32) (*apps.ControllerRevision, error)
	NextRevision(revisions []*apps.ControllerRevision) int64
	GetRevisionLabelSelector(s *appsv1alpha1.SidecarSet) *metav1.LabelSelector
}

type realControl struct {
	Client client.Client
}

func NewHistoryControl(client client.Client) HistoryControl {
	return &realControl{
		Client: client,
	}
}

func (r *realControl) NewRevision(s *appsv1alpha1.SidecarSet, namespace string, revision int64, collisionCount *int32) (
	*apps.ControllerRevision, error,
) {
	patch, err := r.getPatch(s)
	if err != nil {
		return nil, err
	}

	cr, err := history.NewControllerRevision(s,
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
func (r *realControl) getPatch(s *appsv1alpha1.SidecarSet) ([]byte, error) {
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
func (r *realControl) NextRevision(revisions []*apps.ControllerRevision) int64 {
	count := len(revisions)
	if count <= 0 {
		return 1
	}
	return revisions[count-1].Revision + 1
}

func (r *realControl) GetRevisionLabelSelector(s *appsv1alpha1.SidecarSet) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			SidecarSetKindName: s.GetName(),
		},
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
		hash := history.HashControllerRevision(revision, collisionCount)
		// Update the revisions name
		clone.Name = history.ControllerRevisionName(parent.GetName(), hash)
		err := r.Client.Create(context.TODO(), clone)
		if errors.IsAlreadyExists(err) {
			exists := &apps.ControllerRevision{}
			key := types.NamespacedName{
				Namespace: clone.Namespace,
				Name:      clone.Name,
			}
			err = r.Client.Get(context.TODO(), key, exists)
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

func copySidecarSetSpecRevision(dst, src map[string]interface{}) {
	// we will use patch instead of update operation to update pods in the future
	// dst["$patch"] = "replace"
	// only record these revisions
	dst["volumes"] = src["volumes"]
	dst["containers"] = src["containers"]
	dst["initContainers"] = src["initContainers"]
	dst["imagePullSecrets"] = src["imagePullSecrets"]
}
