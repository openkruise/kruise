package expectations

import (
	"testing"
)

func TestScale(t *testing.T) {
	e := NewScaleExpectations()
	controllerCreateKey := "default"
	podCreate := "podCreate"
	e.ExpectScale(controllerCreateKey, Create, podCreate)
	if ok, _, _ := e.SatisfiedExpectations(controllerCreateKey); ok {
		t.Fatalf("expected not satisfied")
	}
	e.ObserveScale(controllerCreateKey, Create, podCreate)
	if ok, _, _ := e.SatisfiedExpectations(controllerCreateKey); !ok {
		t.Fatalf("expected not satisfied")
	}

	controllerDeleteKey := "default-delete"
	podDelete := "podDelete"

	e.ExpectScale(controllerDeleteKey, Delete, podDelete)
	if ok, _, _ := e.SatisfiedExpectations(controllerDeleteKey); ok {
		t.Fatalf("expected not satisfied")
	}
	e.ObserveScale(controllerDeleteKey, Delete, podDelete)
	if ok, _, _ := e.SatisfiedExpectations(controllerDeleteKey); !ok {
		t.Fatalf("expected not satisfied")
	}
}
