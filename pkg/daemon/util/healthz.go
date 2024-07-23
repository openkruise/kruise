/*
Copyright 2021 The Kruise Authors.

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

package util

import (
	"fmt"
	"net/http"
	"sync"

	"k8s.io/klog/v2"
)

// HealthCheckFunc is the interface theat a checker should impl.
type HealthCheckFunc func(req *http.Request) error

// HealthCheck defines the interface that a checker should impl.
type HealthCheck interface {
	Check(req *http.Request) error
}

// Healthz is a manager which will run all healthChecks registered.
type Healthz struct {
	sync.Mutex
	checks       map[string]HealthCheck
	info         string
	healthzCount int
}

// NewHealthz create a Healthz
func NewHealthz() *Healthz {
	return &Healthz{
		checks: make(map[string]HealthCheck),
	}
}

// Handler implements the http handler
func (h *Healthz) Handler(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	defer h.Unlock()
	h.healthzCount++
	for name, handler := range h.checks {
		if err := handler.Check(r); err != nil {
			w.WriteHeader(500)
			errMsg := fmt.Sprintf("check %v failed, err: %v", name, err)
			_, _ = w.Write([]byte(errMsg))
			klog.InfoS("/healthz", "message", errMsg)
			return
		}
	}

	w.WriteHeader(200)
	_, _ = w.Write([]byte("ok"))
	if h.healthzCount%10 == 0 {
		klog.V(6).InfoS("/healthz ok", "message", h.info)
	}
}

// Register a check handler
func (h *Healthz) Register(name string, handler HealthCheck) {
	h.Lock()
	defer h.Unlock()
	h.checks[name] = handler
}

// RegisterFunc a check handlerFunc
func (h *Healthz) RegisterFunc(name string, handler HealthCheckFunc) {
	h.Lock()
	defer h.Unlock()
	h.checks[name] = &healthChecker{f: handler}
}

// Unregister a check handler
func (h *Healthz) Unregister(name string) {
	h.Lock()
	defer h.Unlock()
	delete(h.checks, name)
}

// SetInfo set the information when /heathz returns
func (h *Healthz) SetInfo(info string) {
	h.info = info
}

type healthChecker struct {
	f HealthCheckFunc
}

func (h *healthChecker) Check(req *http.Request) error {
	return h.f(req)
}
