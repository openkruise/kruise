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

package expectations

import (
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

// UpdateExpectations is an interface that allows users to set and wait on expectations of pods update.
type UpdateExpectations interface {
	ExpectUpdated(controllerKey, revision string, obj metav1.Object)
	ObserveUpdated(controllerKey, revision string, obj metav1.Object)
	DeleteObject(controllerKey string, obj metav1.Object)
	SatisfiedExpectations(controllerKey, revision string) (bool, time.Duration, []string)
	DeleteExpectations(controllerKey string)
}

// NewUpdateExpectations returns a common UpdateExpectations.
func NewUpdateExpectations(revisionAdapter revisionadapter.Interface) UpdateExpectations {
	return &realUpdateExpectations{
		controllerCache: make(map[string]*realControllerUpdateExpectations),
		revisionAdapter: revisionAdapter,
	}
}

type realUpdateExpectations struct {
	sync.Mutex
	// key: parent key, workload namespace/name
	controllerCache map[string]*realControllerUpdateExpectations
	// the impl of interface
	revisionAdapter revisionadapter.Interface
}

type realControllerUpdateExpectations struct {
	// latest revision
	revision string
	// item: pod name for this revision
	objsUpdated               sets.String
	firstUnsatisfiedTimestamp time.Time
}

func (r *realUpdateExpectations) ExpectUpdated(controllerKey, revision string, obj metav1.Object) {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil || expectations.revision != revision {
		expectations = &realControllerUpdateExpectations{
			revision:    revision,
			objsUpdated: sets.NewString(),
		}
		r.controllerCache[controllerKey] = expectations
	}

	expectations.objsUpdated.Insert(getKey(obj))
}

func (r *realUpdateExpectations) ObserveUpdated(controllerKey, revision string, obj metav1.Object) {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil {
		return
	}

	if expectations.revision == revision && expectations.objsUpdated.Has(getKey(obj)) && r.revisionAdapter.EqualToRevisionHash(controllerKey, obj, revision) {
		expectations.objsUpdated.Delete(getKey(obj))
	}

	if expectations.revision != revision || expectations.objsUpdated.Len() == 0 {
		delete(r.controllerCache, controllerKey)
	}
}

func (r *realUpdateExpectations) DeleteObject(controllerKey string, obj metav1.Object) {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil {
		return
	}

	expectations.objsUpdated.Delete(getKey(obj))
}

func (r *realUpdateExpectations) SatisfiedExpectations(controllerKey, revision string) (bool, time.Duration, []string) {
	r.Lock()
	defer r.Unlock()

	oldExpectations := r.controllerCache[controllerKey]
	if oldExpectations == nil {
		return true, 0, nil
	} else if oldExpectations.revision != revision {
		oldExpectations.firstUnsatisfiedTimestamp = time.Time{}
		return true, 0, nil
	}

	if oldExpectations.objsUpdated.Len() > 0 {
		if oldExpectations.firstUnsatisfiedTimestamp.IsZero() {
			oldExpectations.firstUnsatisfiedTimestamp = time.Now()
		}
		return false, time.Since(oldExpectations.firstUnsatisfiedTimestamp), oldExpectations.objsUpdated.List()
	}

	oldExpectations.firstUnsatisfiedTimestamp = time.Time{}
	return true, 0, oldExpectations.objsUpdated.List()
}

func (r *realUpdateExpectations) DeleteExpectations(controllerKey string) {
	r.Lock()
	defer r.Unlock()
	delete(r.controllerCache, controllerKey)
}

func getKey(obj metav1.Object) string {
	return obj.GetNamespace() + "/" + obj.GetName()
}
