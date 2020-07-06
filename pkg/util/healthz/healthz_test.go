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

package healthz

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstallHandler(t *testing.T) {
	mux := http.NewServeMux()
	InstallHandler(mux)

	req, err := http.NewRequest(http.MethodGet, "http://localhost/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected %v, got %v", http.StatusOK, w.Code)
	}
	c := w.Header().Get("Content-Type")
	expectContentType := "text/plain; charset=utf-8"
	if c != expectContentType {
		t.Errorf("expected %v, got %v", expectContentType, c)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected %v, got %v", "ok", w.Body.String())
	}
}

func TestMultiCheckers(t *testing.T) {
	tests := []struct {
		path             string
		expectedResponse string
		expectedStatus   int
		addBadCheck      bool
	}{
		{"?verbose", "[+]ping ok\nhealthz check passed\n", http.StatusOK, false},
		{"?verbose=true", "[+]ping ok\nhealthz check passed\n", http.StatusOK, false},
		{"?verbose", "[+]ping ok\n[-]bad failed: reason withheld\nhealthz check failed\n", http.StatusInternalServerError, true},
		{"/ping", "ok", http.StatusOK, false},
		{"", "ok", http.StatusOK, false},
		{"/ping", "ok", http.StatusOK, true},
		{"/bad", "internal server error: this will fail\n", http.StatusInternalServerError, true},
		{"", "[+]ping ok\n[-]bad failed: reason withheld\nhealthz check failed\n", http.StatusInternalServerError, true},
	}

	for i, test := range tests {
		mux := http.NewServeMux()
		checks := []HealthChecker{PingHealthz}
		if test.addBadCheck {
			checks = append(checks, NamedChecker("bad", func(_ *http.Request) error {
				return errors.New("this will fail")
			}))
		}
		InstallHandler(mux, checks...)
		path := "/healthz"
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost%s%v", path, test.path), nil)
		if err != nil {
			t.Fatalf("case[%d] unexpected error: %v", i, err)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != test.expectedStatus {
			t.Errorf("case[%d] expected: %v, got: %v", i, test.expectedStatus, w.Code)
		}
		c := w.Header().Get("Content-Type")
		contentType := "text/plain; charset=utf-8"
		if c != contentType {
			t.Errorf("case[%d] expected: %v, got: %v", i, contentType, c)
		}
		if w.Body.String() != test.expectedResponse {
			t.Errorf("case[%d] expected:\n%v\ngot:\n%v\n", i, test.expectedResponse, w.Body.String())
		}
	}
}
