package util

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockHealthChecker struct {
	shouldFail bool
	failMsg    string
}

func (m *mockHealthChecker) Check(_ *http.Request) error {
	if m.shouldFail {
		return errors.New(m.failMsg)
	}
	return nil
}

func TestNewHealthz(t *testing.T) {
	h := NewHealthz()
	if h == nil {
		t.Fatal("NewHealthz() returned nil")
	}
	if h.checks == nil {
		t.Error("NewHealthz().checks should not be nil")
	}
	if len(h.checks) != 0 {
		t.Errorf("Expected 0 checks, got %d", len(h.checks))
	}
}

func TestHandler_NoChecks(t *testing.T) {
	h := NewHealthz()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)

	h.Handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Body.String(); got != "ok" {
		t.Errorf("expected body %q, got %q", "ok", got)
	}
}

func TestHandler_SingleSuccessfulCheck(t *testing.T) {
	h := NewHealthz()
	h.Register("always-ok", &mockHealthChecker{shouldFail: false})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	h.Handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if got := rr.Body.String(); got != "ok" {
		t.Errorf("expected body %q, got %q", "ok", got)
	}
}

func TestHandler_SingleFailingCheck(t *testing.T) {
	h := NewHealthz()
	failMsg := "boom!"
	h.Register("will-fail", &mockHealthChecker{shouldFail: true, failMsg: failMsg})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	h.Handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	expected := fmt.Sprintf("check %s failed, err: %v", "will-fail", failMsg)
	if got := rr.Body.String(); !strings.Contains(got, expected) {
		t.Errorf("expected body to contain %q, got %q", expected, got)
	}
}

func TestHandler_MultipleChecks_OneFails(t *testing.T) {
	h := NewHealthz()
	h.Register("always-ok", &mockHealthChecker{shouldFail: false})
	h.Register("will-fail", &mockHealthChecker{shouldFail: true, failMsg: "fail"})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	h.Handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}

func TestRegisterUnregister(t *testing.T) {
	h := NewHealthz()
	name := "my-check"
	h.Register(name, &mockHealthChecker{})
	if _, ok := h.checks[name]; !ok {
		t.Errorf("check %q was not registered", name)
	}
	h.Unregister(name)
	if _, ok := h.checks[name]; ok {
		t.Errorf("check %q was not unregistered", name)
	}
}

func TestRegisterFunc(t *testing.T) {
	h := NewHealthz()
	name := "func-ok"
	h.RegisterFunc(name, func(*http.Request) error { return nil })

	rr1 := httptest.NewRecorder()
	h.Handler(rr1, httptest.NewRequest("GET", "/healthz", nil))
	if rr1.Code != http.StatusOK {
		t.Errorf("after RegisterFunc(ok), expected 200, got %d", rr1.Code)
	}

	h.RegisterFunc("func-fail", func(*http.Request) error { return errors.New("bad") })
	rr2 := httptest.NewRecorder()
	h.Handler(rr2, httptest.NewRequest("GET", "/healthz", nil))
	if rr2.Code != http.StatusInternalServerError {
		t.Errorf("after RegisterFunc(fail), expected 500, got %d", rr2.Code)
	}
}

func TestSetInfoAndTenthCallCounter(t *testing.T) {
	h := NewHealthz()
	h.SetInfo("extra-info")

	// Call 10 times to hit the V(6) log branch and increment counter
	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		h.Handler(rr, httptest.NewRequest("GET", "/healthz", nil))
		if rr.Code != http.StatusOK {
			t.Errorf("iteration %d: expected 200, got %d", i, rr.Code)
		}
	}
	// internal counter should now be 10
	if h.healthzCount != 10 {
		t.Errorf("expected healthzCount to be 10; got %d", h.healthzCount)
	}
}
