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

package expectations

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeRevisionAdapterImpl struct{}

func (r *fakeRevisionAdapterImpl) EqualToRevisionHash(_ string, obj metav1.Object, hash string) bool {
	return obj.GetLabels()["revision"] == hash
}

func (r *fakeRevisionAdapterImpl) WriteRevisionHash(obj metav1.Object, hash string) {
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string, 1))
	}
	obj.GetLabels()["revision"] = hash
}

func TestUpdate(t *testing.T) {
	controllerKey := "default/controller-test"
	revisions := []string{"rev-0", "rev-1"}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			Labels: map[string]string{
				"revision": "none",
			},
		},
	}
	c := NewUpdateExpectations(&fakeRevisionAdapterImpl{})

	// no pod in cache
	if satisfied, _, _ := c.SatisfiedExpectations(controllerKey, revisions[0]); !satisfied {
		t.Fatalf("expected no pod for revision %v", revisions[0])
	}

	// update to rev-0 and then observed
	tmpPod := pod.DeepCopy()
	c.ExpectUpdated(controllerKey, revisions[0], tmpPod)
	if satisfied, _, _ := c.SatisfiedExpectations(controllerKey, revisions[0]); satisfied {
		t.Fatalf("expected pod updated for revision %v, got false", revisions[0])
	}

	tmpPod.Labels["revision"] = revisions[0]
	c.ObserveUpdated(controllerKey, revisions[0], tmpPod)
	if satisfied, _, _ := c.SatisfiedExpectations(controllerKey, revisions[0]); !satisfied {
		t.Fatalf("expected no pod for revision %v", revisions[0])
	}

	// rev-0 up to rev-1
	c.ExpectUpdated(controllerKey, revisions[0], tmpPod)
	if satisfied, _, _ := c.SatisfiedExpectations(controllerKey, revisions[1]); !satisfied {
		t.Fatalf("expected cache clean when revision updated")
	}
	tmpPod.Labels["revision"] = revisions[1]
	c.ObserveUpdated(controllerKey, revisions[0], tmpPod)
	if satisfied, _, _ := c.SatisfiedExpectations(controllerKey, revisions[1]); !satisfied {
		t.Fatalf("expected cache clean when revision updated")
	}

	// delete controllerKey
	c.DeleteExpectations(controllerKey)
	if satisfied, _, _ := c.SatisfiedExpectations(controllerKey, revisions[1]); !satisfied {
		t.Fatalf("expected no pod for revision %v", revisions[0])
	}

}
