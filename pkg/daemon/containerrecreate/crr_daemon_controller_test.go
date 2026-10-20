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

package containerrecreate

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestEnqueueAfterDelete(t *testing.T) {
	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "crr-1",
		},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{PodName: "pod-1"},
	}

	tests := []struct {
		name    string
		obj     interface{}
		wantKey string
	}{
		{
			name:    "delete event",
			obj:     crr,
			wantKey: "default/pod-1",
		},
		{
			name: "delete tombstone",
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/crr-1",
				Obj: crr,
			},
			wantKey: "default/pod-1",
		},
		{
			name: "unexpected delete object",
			obj:  struct{}{},
		},
		{
			name: "tombstone containing unexpected object",
			obj: cache.DeletedFinalStateUnknown{
				Key: "default/not-a-crr",
				Obj: struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := workqueue.New()
			defer queue.ShutDown()

			enqueueAfterDelete(queue, tt.obj)

			if tt.wantKey == "" {
				if queue.Len() != 0 {
					t.Fatalf("expected no queued key, got %d", queue.Len())
				}
				return
			}
			if queue.Len() != 1 {
				t.Fatalf("expected one queued key, got %d", queue.Len())
			}
			key, shutdown := queue.Get()
			if shutdown {
				t.Fatal("queue unexpectedly shut down")
			}
			defer queue.Done(key)
			if key != tt.wantKey {
				t.Fatalf("expected key %s, got %v", tt.wantKey, key)
			}
		})
	}
}
