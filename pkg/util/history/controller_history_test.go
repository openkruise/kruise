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

package history

import (
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRevisionHistory(t *testing.T) {
	var collisionCount int32 = 10
	var revisionNum int64 = 5
	parent := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "parent-cs",
			UID:       uuid.NewUUID(),
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-app",
					},
				},
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
			CollisionCount: &collisionCount,
		},
	}

	cr, err := history.NewControllerRevision(parent,
		appsv1alpha1.GroupVersion.WithKind("CloneSet"),
		parent.Spec.Template.Labels,
		runtime.RawExtension{Raw: []byte(`{}`)},
		revisionNum,
		parent.Status.CollisionCount)
	if err != nil {
		t.Fatalf("Failed to new controller revision: %v", err)
	}

	fakeClient := fake.NewClientBuilder().Build()
	historyControl := NewHistory(fakeClient)

	newCR, err := historyControl.CreateControllerRevision(parent, cr, parent.Status.CollisionCount)
	if err != nil {
		t.Fatalf("Failed to create controller revision: %v", err)
	}

	expectedName := parent.Name + "-" + history.HashControllerRevision(cr, parent.Status.CollisionCount)
	if newCR.Name != expectedName {
		t.Fatalf("Expected ControllerRevision name %v, got %v", expectedName, newCR.Name)
	}

	selector, _ := util.ValidatedLabelSelectorAsSelector(parent.Spec.Selector)
	gotRevisions, err := historyControl.ListControllerRevisions(parent, selector)
	if err != nil {
		t.Fatalf("Failed to list revisions: %v", err)
	}

	expectedRevisions := []*apps.ControllerRevision{newCR}
	if !reflect.DeepEqual(expectedRevisions, gotRevisions) {
		t.Fatalf("List unexpected revisions")
	}

	newCR, err = historyControl.UpdateControllerRevision(newCR, revisionNum+1)
	if err != nil {
		t.Fatalf("Failed to update revision: %v", err)
	}

	if newCR.Revision != revisionNum+1 {
		t.Fatalf("Expected revision %v, got %v", revisionNum+1, newCR.Revision)
	}

	if err = historyControl.DeleteControllerRevision(newCR); err != nil {
		t.Fatalf("Failed to delete revision: %v", err)
	}

	gotRevisions, err = historyControl.ListControllerRevisions(parent, selector)
	if err != nil {
		t.Fatalf("Failed to list revisions: %v", err)
	}
	if len(gotRevisions) > 0 {
		t.Fatalf("Expected no ControllerRevision left, got %v", gotRevisions)
	}
}
