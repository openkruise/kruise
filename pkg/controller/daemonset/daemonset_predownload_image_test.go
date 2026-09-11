/*
Copyright 2026 The Kruise Authors.

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

package daemonset

import (
	"fmt"
	"testing"

	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func revisionOf(name, raw string) *apps.ControllerRevision {
	return &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       runtime.RawExtension{Raw: []byte(raw)},
	}
}

func setRestartableInitContainerGate(t *testing.T, enabled bool) {
	t.Helper()

	if err := utilfeature.DefaultMutableFeatureGate.Set(
		fmt.Sprintf("%s=%v", features.InPlaceUpdateRestartableInitContainer, enabled)); err != nil {
		t.Fatalf("failed to set feature-gate: %v", err)
	}
}

// TestDiffImagesBetweenRevisionsForRestartableInitContainer makes sure the images of the
// restartable init containers (native sidecar containers) are pre-downloaded as well.
// Advanced DaemonSet uses the default CalculateSpec of the inplaceupdate package, so it does
// in-place update such containers and therefore has to pre-download their images too.
func TestDiffImagesBetweenRevisionsForRestartableInitContainer(t *testing.T) {
	const (
		// "sidecar" is restartable, "setup" is a regular init container
		oldRaw = `{"spec":{"template":{"spec":{"containers":[{"name":"main","image":"main1"}],"initContainers":[{"name":"setup","image":"setup1"},{"name":"sidecar","image":"foo1","restartPolicy":"Always"}]}}}}`
		newRaw = `{"spec":{"template":{"spec":{"containers":[{"name":"main","image":"main1"}],"initContainers":[{"name":"setup","image":"setup2"},{"name":"sidecar","image":"foo2","restartPolicy":"Always"}]}}}}`
		// an older revision where the sidecar image is yet another one
		olderRaw = `{"spec":{"template":{"spec":{"containers":[{"name":"main","image":"main1"}],"initContainers":[{"name":"setup","image":"setup1"},{"name":"sidecar","image":"foo0","restartPolicy":"Always"}]}}}}`
	)

	cases := []struct {
		name         string
		gateEnabled  bool
		oldRevisions []*apps.ControllerRevision
		expect       map[string]string
	}{
		{
			name:         "gate disabled keeps the previous behavior",
			gateEnabled:  false,
			oldRevisions: []*apps.ControllerRevision{revisionOf("old", oldRaw)},
			expect:       map[string]string{},
		},
		{
			name:         "only the restartable init container image is pre-downloaded",
			gateEnabled:  true,
			oldRevisions: []*apps.ControllerRevision{revisionOf("old", oldRaw)},
			expect:       map[string]string{"sidecar": "foo2"},
		},
		{
			name:        "differs from any of the old revisions",
			gateEnabled: true,
			oldRevisions: []*apps.ControllerRevision{
				revisionOf("old", oldRaw),
				revisionOf("older", olderRaw),
			},
			expect: map[string]string{"sidecar": "foo2"},
		},
		{
			name:         "no old revision pre-downloads everything",
			gateEnabled:  true,
			oldRevisions: nil,
			expect:       map[string]string{"main": "main1", "sidecar": "foo2"},
		},
	}

	defer setRestartableInitContainerGate(t, false)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRestartableInitContainerGate(t, tc.gateEnabled)

			got := diffImagesBetweenRevisions(tc.oldRevisions, revisionOf("new", newRaw))
			if len(got) != len(tc.expect) {
				t.Fatalf("expected %v, got %v", tc.expect, got)
			}
			for k, v := range tc.expect {
				if got[k] != v {
					t.Fatalf("expected %v, got %v", tc.expect, got)
				}
			}
		})
	}
}
