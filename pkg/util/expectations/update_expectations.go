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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// UpdateExpectations is an interface that allows users to set and wait on expectations of pods update.
type UpdateExpectations interface {
	ExpectUpdated(controllerKey, revision string, obj metav1.Object)
	ObserveUpdated(controllerKey, revision string, obj metav1.Object)
	SatisfiedExpectations(controllerKey, revision string) (bool, []string)
	DeleteExpectations(controllerKey string)
}

// NewUpdateExpectations returns a common UpdateExpectations.
func NewUpdateExpectations(getRevision func(metav1.Object) string) UpdateExpectations {
	return &realUpdateExpectations{
		controllerCache: make(map[string]*realControllerUpdateExpectations),
		getRevision:     getRevision,
	}
}

type realUpdateExpectations struct {
	sync.RWMutex
	// key: parent key, workload namespace/name
	controllerCache map[string]*realControllerUpdateExpectations
	// how to get pod revision
	getRevision func(metav1.Object) string
}

type realControllerUpdateExpectations struct {
	// latest revision
	revision string
	// item: pod name for this revision
	objsUpdated sets.String
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

	if expectations.revision == revision && expectations.objsUpdated.Has(getKey(obj)) && r.getRevision(obj) == revision {
		expectations.objsUpdated.Delete(getKey(obj))
	}

	if expectations.revision != revision || expectations.objsUpdated.Len() == 0 {
		delete(r.controllerCache, controllerKey)
	}
}

func (r *realUpdateExpectations) SatisfiedExpectations(controllerKey, revision string) (bool, []string) {
	r.Lock()
	defer r.Unlock()

	oldExpectations := r.controllerCache[controllerKey]
	if oldExpectations == nil {
		return true, nil
	} else if oldExpectations.revision != revision {
		return true, nil
	}

	return oldExpectations.objsUpdated.Len() == 0, oldExpectations.objsUpdated.List()
}

func (r *realUpdateExpectations) DeleteExpectations(controllerKey string) {
	r.Lock()
	defer r.Unlock()
	delete(r.controllerCache, controllerKey)
}

func getKey(obj metav1.Object) string {
	return obj.GetNamespace() + "/" + obj.GetName()
}
