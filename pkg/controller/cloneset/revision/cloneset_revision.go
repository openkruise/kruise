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

package revision

import (
	"encoding/json"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/volumeclaimtemplate"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/controller/history"
)

var (
	patchCodec = scheme.Codecs.LegacyCodec(appsv1alpha1.SchemeGroupVersion)
)

// Interface is a interface to new and apply ControllerRevision.
type Interface interface {
	NewRevision(cs *appsv1alpha1.CloneSet, revision int64, collisionCount *int32) (*apps.ControllerRevision, error)
	ApplyRevision(cs *appsv1alpha1.CloneSet, revision *apps.ControllerRevision) (*appsv1alpha1.CloneSet, error)
}

// NewRevisionControl create a normal revision control.
func NewRevisionControl() Interface {
	return &realControl{}
}

type realControl struct {
}

func (c *realControl) NewRevision(cs *appsv1alpha1.CloneSet, revision int64, collisionCount *int32) (*apps.ControllerRevision, error) {
	coreControl := clonesetcore.New(cs)
	patch, err := c.getPatch(cs, coreControl)
	if err != nil {
		return nil, err
	}
	cr, err := history.NewControllerRevision(cs,
		clonesetutils.ControllerKind,
		cs.Spec.Template.Labels,
		runtime.RawExtension{Raw: patch},
		revision,
		collisionCount)
	if err != nil {
		return nil, err
	}
	if cr.ObjectMeta.Annotations == nil {
		cr.ObjectMeta.Annotations = make(map[string]string)
	}
	for key, value := range cs.Annotations {
		cr.ObjectMeta.Annotations[key] = value
	}
	volumeclaimtemplate.PatchVCTemplateHash(cr, cs.Spec.VolumeClaimTemplates)
	return cr, nil
}

// getPatch returns a strategic merge patch that can be applied to restore a CloneSet to a
// previous version. If the returned error is nil the patch is valid. The current state that we save is just the
// PodSpecTemplate. We can modify this later to encompass more state (or less) and remain compatible with previously
// recorded patches.
func (c *realControl) getPatch(cs *appsv1alpha1.CloneSet, coreControl clonesetcore.Control) ([]byte, error) {
	str, err := runtime.Encode(patchCodec, cs)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	_ = json.Unmarshal(str, &raw)
	objCopy := make(map[string]interface{})
	specCopy := make(map[string]interface{})
	spec := raw["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})

	coreControl.SetRevisionTemplate(specCopy, template)
	objCopy["spec"] = specCopy
	patch, err := json.Marshal(objCopy)
	return patch, err
}

func (c *realControl) ApplyRevision(cs *appsv1alpha1.CloneSet, revision *apps.ControllerRevision) (*appsv1alpha1.CloneSet, error) {
	clone := cs.DeepCopy()
	cloneBytes, err := runtime.Encode(patchCodec, clone)
	if err != nil {
		return nil, err
	}
	patched, err := strategicpatch.StrategicMergePatch(cloneBytes, revision.Data.Raw, clone)
	if err != nil {
		return nil, err
	}
	coreControl := clonesetcore.New(clone)
	return coreControl.ApplyRevisionPatch(patched)
}
