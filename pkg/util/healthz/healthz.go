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
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/klog"
)

// HealthChecker is named healthz checker
type HealthChecker interface {
	Name() string
	Check(r *http.Request) error
}

// PingHealthz return true always when checked
var PingHealthz HealthChecker = ping{}

type ping struct{}

func (ping) Name() string { return "ping" }

func (ping) Check(_ *http.Request) error { return nil }

var _ HealthChecker = &healthzCheck{}

type healthzCheck struct {
	name  string
	check func(r *http.Request) error
}

func (c *healthzCheck) Name() string { return c.name }

func (c *healthzCheck) Check(r *http.Request) error { return c.check(r) }

func NamedChecker(name string, check func(r *http.Request) error) HealthChecker {
	return &healthzCheck{
		name:  name,
		check: check,
	}
}

// Mux is an HTTP request multiplexer
type Mux interface {
	Handle(pattern string, handler http.Handler)
}

// InstallHandler registers handlers for health checking on the path "/healthz" to mux
func InstallHandler(mux Mux, checks ...HealthChecker) {
	installPathHandler(mux, "/healthz", checks...)
}

// InstallReadyzHandler registers handlers for health checking on the path "/readyz" to mux
func InstallReadyzHandler(mux Mux, checks ...HealthChecker) {
	installPathHandler(mux, "/readyz", checks...)
}

// InstallLivezHandler registers handlers for liveness checking on the path "/livez" to mux
func InstallLivezHandler(mux Mux, checks ...HealthChecker) {
	installPathHandler(mux, "/livez", checks...)
}

func installPathHandler(mux Mux, path string, checks ...HealthChecker) {
	if len(checks) == 0 {
		klog.V(5).Info("No default health checks specified. Installing the ping handler.")
		checks = []HealthChecker{PingHealthz}
	}
	klog.V(5).Infof("Installing health checkers for (%v): %v", path, formatQuoted(checkerNames(checks...)...))

	mux.Handle(path, healthzHandler(checks...))
	for _, check := range checks {
		mux.Handle(fmt.Sprintf("%s/%s", path, check.Name()), checkHandlerAdaptor(check.Check))
	}
}

func healthzHandler(checks ...HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var verboseOut bytes.Buffer
		var failed bool
		for _, check := range checks {
			if err := check.Check(r); err != nil {
				// Only show failed check. If wants more detail, they should request detailed checks.
				klog.V(4).Infof("healthz check %v failed: %v", check.Name(), err)
				fmt.Fprintf(&verboseOut, "[-]%v failed: reason withheld\n", check.Name())
				failed = true
			} else {
				fmt.Fprintf(&verboseOut, "[+]%v ok\n", check.Name())
			}
		}
		if failed {
			klog.V(2).Infof("%vhealthz check failed", verboseOut.String())
			http.Error(w, fmt.Sprintf("%vhealthz check failed", verboseOut.String()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, found := r.URL.Query()["verbose"]; !found {
			fmt.Fprint(w, "ok")
			return
		}

		verboseOut.WriteTo(w)
		fmt.Fprint(w, "healthz check passed\n")
	}
}

func checkHandlerAdaptor(c func(r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := c(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("internal server error: %v", err), http.StatusInternalServerError)
		} else {
			fmt.Fprintf(w, "ok")
		}
	}
}

func checkerNames(checks ...HealthChecker) []string {
	// accumulate the names of checks for printing them out.
	checkerNames := make([]string, 0, len(checks))
	for _, check := range checks {
		checkerNames = append(checkerNames, check.Name())
	}
	return checkerNames
}

func formatQuoted(names ...string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, ",")
}
