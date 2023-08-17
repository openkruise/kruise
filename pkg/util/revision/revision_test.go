/*
Copyright 2023 The Kruise Authors.

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

package revision

import (
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsPodUpdate(t *testing.T) {
	cases := []struct {
		name    string
		hash    string
		pod     *v1.Pod
		updated bool
	}{
		{
			name: "normal state, long hash, updated",
			hash: "app-name-new",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "app-name-new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: true,
		},
		{
			name: "normal state, long and short hash, updated",
			hash: "app-name-new",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: true,
		},
		{
			name: "normal state, short and long hash, updated",
			hash: "new",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "app-name-new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: true,
		},
		{
			name: "normal state, short hash, updated",
			hash: "new",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: true,
		},
		{
			name: "normal state, long hash, not updated",
			hash: "app-name-old",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "app-name-new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: false,
		},
		{
			name: "normal state, short hash, not updated",
			hash: "old",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: false,
		},
		{
			name: "preparing-update state, old revision, long hash, updated",
			hash: "app-name-old",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "app-name-new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStatePreparingUpdate),
				},
			},
			},
			updated: true,
		},
		{
			name: "preparing-update state, old revision, long and short hash, updated",
			hash: "app-name-old",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStatePreparingUpdate),
				},
			},
			},
			updated: true,
		},
		{
			name: "preparing-update state, old revision, short and long hash, updated",
			hash: "old",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "app-name-new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStatePreparingUpdate),
				},
			},
			},
			updated: true,
		},
		{
			name: "preparing-update state, old revision, short hash, updated",
			hash: "old",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStatePreparingUpdate),
				},
			},
			},
			updated: true,
		},
		{
			name: "preparing-update state, new revision, long hash, updated",
			hash: "app-name-new",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "app-name-new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStatePreparingUpdate),
				},
			},
			},
			updated: true,
		},
		{
			name: "preparing-update state, new vision short hash, updated",
			hash: "new",
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					apps.ControllerRevisionHashLabelKey: "new",
					appspub.LifecycleStateKey:           string(appspub.LifecycleStateNormal),
				},
			},
			},
			updated: true,
		},
	}

	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PreparingUpdateAsUpdate, true)()

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			got := IsPodUpdate(cs.pod, cs.hash)
			if got != cs.updated {
				t.Fatalf("expect %v, but got %v", cs.updated, got)
			}
		})
	}
}
