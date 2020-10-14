package expectations

import (
	"testing"
)

func TestScale(t *testing.T) {
	e := NewScaleExpectations()
	controllerKey01 := "default/cs01"
	controllerKey02 := "default/cs02"
	pod01 := "pod01"
	pod02 := "pod02"

	e.ExpectScale(controllerKey01, Create, pod01)
	e.ExpectScale(controllerKey01, Create, pod02)
	e.ExpectScale(controllerKey01, Delete, pod01)
	if ok, _, _ := e.SatisfiedExpectations(controllerKey01); ok {
		t.Fatalf("expected not satisfied")
	}

	e.ObserveScale(controllerKey01, Create, pod02)
	e.ObserveScale(controllerKey01, Create, pod01)
	if ok, _, _ := e.SatisfiedExpectations(controllerKey01); ok {
		t.Fatalf("expected not satisfied")
	}

	e.ObserveScale(controllerKey02, Delete, pod01)
	if ok, _, _ := e.SatisfiedExpectations(controllerKey01); ok {
		t.Fatalf("expected not satisfied")
	}

	e.ObserveScale(controllerKey01, Delete, pod01)
	if ok, _, _ := e.SatisfiedExpectations(controllerKey01); !ok {
		t.Fatalf("expected satisfied")
	}
}
