package expectations

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

// ScaleAction is the action of scale, like create and delete.
type ScaleAction string

const (
	// Create action
	Create ScaleAction = "create"
	// Delete action
	Delete ScaleAction = "delete"
)

// ScaleExpectations is an interface that allows users to set and wait on expectations of pods scale.
type ScaleExpectations interface {
	ExpectScale(controllerKey string, action ScaleAction, name string)
	ObserveScale(controllerKey string, action ScaleAction, name string)
	SatisfiedExpectations(controllerKey string) (bool, time.Duration, map[ScaleAction][]string)
	DeleteExpectations(controllerKey string)
	GetExpectations(controllerKey string) map[ScaleAction]sets.String
}

// NewScaleExpectations returns a common ScaleExpectations.
func NewScaleExpectations() ScaleExpectations {
	return &realScaleExpectations{
		controllerCache: make(map[string]*realControllerScaleExpectations),
	}
}

type realScaleExpectations struct {
	sync.Mutex
	// key: parent key, workload namespace/name
	controllerCache map[string]*realControllerScaleExpectations
}

type realControllerScaleExpectations struct {
	// item: name for this object
	objsCache                 map[ScaleAction]sets.String
	firstUnsatisfiedTimestamp time.Time
}

func (r *realScaleExpectations) GetExpectations(controllerKey string) map[ScaleAction]sets.String {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil {
		return nil
	}

	res := make(map[ScaleAction]sets.String, len(expectations.objsCache))
	for k, v := range expectations.objsCache {
		res[k] = sets.NewString(v.List()...)
	}

	return res
}

func (r *realScaleExpectations) ExpectScale(controllerKey string, action ScaleAction, name string) {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil {
		expectations = &realControllerScaleExpectations{
			objsCache: make(map[ScaleAction]sets.String),
		}
		r.controllerCache[controllerKey] = expectations
	}

	if s := expectations.objsCache[action]; s != nil {
		s.Insert(name)
	} else {
		expectations.objsCache[action] = sets.NewString(name)
	}
}

func (r *realScaleExpectations) ObserveScale(controllerKey string, action ScaleAction, name string) {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil {
		return
	}

	s := expectations.objsCache[action]
	if s == nil {
		return
	}
	s.Delete(name)

	for _, s := range expectations.objsCache {
		if s.Len() > 0 {
			return
		}
	}
	delete(r.controllerCache, controllerKey)
}

func (r *realScaleExpectations) SatisfiedExpectations(controllerKey string) (bool, time.Duration, map[ScaleAction][]string) {
	r.Lock()
	defer r.Unlock()

	expectations := r.controllerCache[controllerKey]
	if expectations == nil {
		return true, 0, nil
	}

	for a, s := range expectations.objsCache {
		if s.Len() > 0 {
			if expectations.firstUnsatisfiedTimestamp.IsZero() {
				expectations.firstUnsatisfiedTimestamp = time.Now()
			}
			return false, time.Since(expectations.firstUnsatisfiedTimestamp), map[ScaleAction][]string{a: s.List()}
		}
	}

	delete(r.controllerCache, controllerKey)
	return true, 0, nil
}

func (r *realScaleExpectations) DeleteExpectations(controllerKey string) {
	r.Lock()
	defer r.Unlock()
	delete(r.controllerCache, controllerKey)
}
