/*
Copyright 2024 The Kruise Authors.

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

package volumeclaimtemplate

import (
	"encoding/json"
	"hash/fnv"
	"sort"
	"strconv"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
)

const (
	// HashAnnotation represents the specs of volumeclaimtemplates hash
	HashAnnotation = "kruise.io/cloneset-volumeclaimtemplate-hash"
)

var (
	// defaultHasher is default VolumeClaimTemplatesHasher
	defaultHasher VolumeClaimTemplatesHasher
)

type VolumeClaimTemplatesHasher interface {
	// GetExpectHash get hash from volume claim templates
	getExpectHash(templates []v1.PersistentVolumeClaim) uint64
}

type PVCGetter = func(string) *v1.PersistentVolumeClaim

type volumeClaimTemplatesHasher struct{}

func NewVolumeClaimTemplatesHasher() VolumeClaimTemplatesHasher {
	return &volumeClaimTemplatesHasher{}
}

func (h *volumeClaimTemplatesHasher) getExpectHash(templates []v1.PersistentVolumeClaim) uint64 {
	var pvcs []v1.PersistentVolumeClaim
	for i := range templates {

		pvcs = append(pvcs, v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: templates[i].Name,
			},
			Spec: templates[i].Spec,
		})
	}

	sort.SliceStable(pvcs, func(i, j int) bool {
		return pvcs[i].Name < pvcs[j].Name
	})
	return hashVolumeClaimTemplate(pvcs)
}

func hashVolumeClaimTemplate(templates []v1.PersistentVolumeClaim) uint64 {
	hash := fnv.New32a()
	envsJSON, _ := json.Marshal(templates)
	hashutil.DeepHashObject(hash, envsJSON)
	return uint64(hash.Sum32())
}

func GetVCTemplatesHash(revision *apps.ControllerRevision) (string, bool) {
	if len(revision.Annotations) > 0 {
		val, exist := revision.Annotations[HashAnnotation]
		return val, exist
	}
	return "", false
}

func PatchVCTemplateHash(revision *apps.ControllerRevision, vcTemplates []v1.PersistentVolumeClaim) {
	if len(revision.Annotations) == 0 {
		revision.Annotations = make(map[string]string)
	}
	if len(vcTemplates) != 0 {
		// get hash of vct
		vcTemplateHash := defaultHasher.getExpectHash(vcTemplates)
		revision.Annotations[HashAnnotation] = strconv.FormatUint(vcTemplateHash, 10)
	} else {
		revision.Annotations[HashAnnotation] = ""
	}
}

// CanVCTemplateInplaceUpdate
func CanVCTemplateInplaceUpdate(oldRevision, newRevision *apps.ControllerRevision) bool {
	newVCTemplatesHash, newExist := GetVCTemplatesHash(newRevision)
	oldVCTemplatesHash, oldExist := GetVCTemplatesHash(oldRevision)
	// oldExist = false, pass this case to avoid unnecessary recreate when first open this feature
	// newExist = false, the hash must exist in normal case, however, corner case may exist:
	// 1. cloneset A with image 1 vct size 1Gi
	// 2. kriuse update to new version, next revision will patch hash
	// 3. 1st update cloneset A with image2 vct size 1Gi -> in-place update and revision hash is not nil
	// 4. 1st update over
	// 5. kruise rollback, next revision will not patch hash
	// 6. 2rd update cloneset A with image3 vct size 1Gi, partition 50% -> in-place update and revision hash is nil
	// 7. kruise re-update
	// 8. 2rd update, with partition 0
	//
	// In step8 of this case, new revision hash does not exist and old revision exists.
	// It might be necessary to include a case where the new revision is non-existent as well.
	if !oldExist || !newExist {
		return true
	}
	return newVCTemplatesHash == oldVCTemplatesHash
}

func init() {
	defaultHasher = NewVolumeClaimTemplatesHasher()
}
