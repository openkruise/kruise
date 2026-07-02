package expectations

import (
	"testing"
)

func TestDeleteExpectations(t *testing.T) {
	e := NewScaleExpectations()
	key := "default/cs-delete"
	pod := "pod01"

	// No-op delete on missing key should not panic
	e.DeleteExpectations(key)

	// Add expectation then delete
	e.ExpectScale(key, Create, pod)
	if ok, _, _ := e.SatisfiedExpectations(key); ok {
		t.Fatal("expected unsatisfied before delete")
	}
	e.DeleteExpectations(key)
	if ok, _, _ := e.SatisfiedExpectations(key); !ok {
		t.Fatal("expected satisfied after DeleteExpectations")
	}
}

func TestGetExpectations(t *testing.T) {
	e := NewScaleExpectations()
	key := "default/cs-get"
	pod1 := "pod01"
	pod2 := "pod02"

	// No entry yet
	if got := e.GetExpectations(key); got != nil {
		t.Fatalf("expected nil for unknown key, got %v", got)
	}

	// Add create expectations
	e.ExpectScale(key, Create, pod1)
	e.ExpectScale(key, Create, pod2)
	e.ExpectScale(key, Delete, pod1)

	got := e.GetExpectations(key)
	if got == nil {
		t.Fatal("expected non-nil expectations map")
	}
	if !got[Create].Has(pod1) || !got[Create].Has(pod2) {
		t.Fatalf("expected create set to contain %s and %s, got %v", pod1, pod2, got[Create])
	}
	if !got[Delete].Has(pod1) {
		t.Fatalf("expected delete set to contain %s, got %v", pod1, got[Delete])
	}

	// GetExpectations should return a copy; mutating it should not affect the original
	got[Create].Delete(pod1)
	original := e.GetExpectations(key)
	if !original[Create].Has(pod1) {
		t.Fatal("GetExpectations should return a copy; mutating the result affected the source")
	}
}

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
