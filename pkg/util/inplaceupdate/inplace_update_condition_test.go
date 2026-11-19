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

package inplaceupdate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

// TestUpdateConditionPatchesStatusOnly asserts updateCondition patches only status.conditions,
// so a full-status write can't clobber sibling status fields kruise's older vendored types don't
// know (e.g. the DRA fields). Checked at the wire level: the typed fake client can't hold such a
// field, so it can't reproduce the clobber directly. See kubernetes/kubernetes#139772.
func TestUpdateConditionPatchesStatusOnly(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "p1", ResourceVersion: "42"},
		Spec: v1.PodSpec{
			ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}},
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}},
		},
	}

	var gotSub string
	var gotType types.PatchType
	var gotData []byte
	cli := fake.NewClientBuilder().WithObjects(pod).WithInterceptorFuncs(interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			gotSub = subResourceName
			gotType = patch.Type()
			data, err := patch.Data(obj)
			if err != nil {
				return err
			}
			gotData = data
			return nil
		},
	}).Build()

	ctrl := New(cli, revisionadapter.NewDefaultImpl()).(*realControl)
	cond := v1.PodCondition{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue, LastTransitionTime: metav1.Now()}
	if err := ctrl.updateCondition(pod, cond); err != nil {
		t.Fatalf("updateCondition returned error: %v", err)
	}

	if gotData == nil {
		t.Fatal("expected updateCondition to issue a status subresource patch, but none was recorded")
	}
	if gotSub != "status" {
		t.Fatalf("expected patch on the status subresource, got %q", gotSub)
	}
	if gotType != types.StrategicMergePatchType {
		t.Fatalf("expected a strategic-merge patch, got %q", gotType)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(gotData, &body); err != nil {
		t.Fatalf("patch body is not valid JSON: %v (%s)", err, gotData)
	}
	status, ok := body["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("patch body has no status section: %s", gotData)
	}
	if _, ok := status["conditions"]; !ok {
		t.Fatalf("patch does not set status.conditions: %s", gotData)
	}
	// Optimistic lock must ride along (#2274): assert the resourceVersion precondition is present.
	if meta, _ := body["metadata"].(map[string]interface{}); meta == nil || meta["resourceVersion"] != "42" {
		t.Errorf("patch is missing the optimistic-lock metadata.resourceVersion=42: %s", gotData)
	}
	// Any status key other than conditions (+ its "$setElementOrder/conditions" directive)
	// could clear fields absent from kruise's vendored types.
	for k := range status {
		if !strings.Contains(k, "conditions") {
			t.Errorf("patch touches unexpected status field %q, which risks clobbering fields absent from kruise's vendored types; body=%s", k, gotData)
		}
	}
}
