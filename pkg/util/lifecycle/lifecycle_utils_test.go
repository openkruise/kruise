/*
Copyright 2025 The Kruise Authors.

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

package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util/podreadiness"
)

type fakeAdapter struct {
	getPod             *corev1.Pod
	updatedPod         *corev1.Pod
	getCalled          bool
	updateCalled       bool
	updateStatusCalled bool
	err                error
}

func (f *fakeAdapter) UpdatePod(pod *corev1.Pod) (*corev1.Pod, error) {
	f.updateCalled = true
	f.updatedPod = pod
	return pod, f.err
}

func (f *fakeAdapter) GetPod(namespace, name string) (*corev1.Pod, error) {
	if namespace == "" || name == "" {
		return nil, errors.New(fmt.Sprintf("GetPod namespace or name is empty, namespace: %s, name: %s", namespace, name))
	}

	pod := &corev1.Pod{}
	f.getCalled = true
	f.getPod = pod
	return pod, f.err
}

func (f *fakeAdapter) UpdatePodStatus(_ *corev1.Pod) error {
	f.updateStatusCalled = true
	return f.err
}

type fakePatchAdapter struct {
	fakeAdapter
	patchedPod          *corev1.Pod
	patchCalled         bool
	patchResourceCalled bool
}

func (f *fakePatchAdapter) PatchPod(pod *corev1.Pod, _ client.Patch) (*corev1.Pod, error) {
	f.patchCalled = true
	f.patchedPod = pod
	return pod, f.err
}
func (f *fakePatchAdapter) PatchPodResource(pod *corev1.Pod, _ client.Patch) (*corev1.Pod, error) {
	f.patchResourceCalled = true
	return pod, f.err
}

type fakePodReadinessControl struct {
	addCalled             bool
	removeCalled          bool
	containsReadinessGate bool
	err                   error
}

func (f *fakePodReadinessControl) AddNotReadyKey(_ *corev1.Pod, _ podreadiness.Message) error {
	f.addCalled = true
	return f.err
}
func (f *fakePodReadinessControl) RemoveNotReadyKey(_ *corev1.Pod, _ podreadiness.Message) error {
	f.removeCalled = true
	return f.err
}
func (f *fakePodReadinessControl) ContainsReadinessGate(_ *corev1.Pod) bool {
	return f.containsReadinessGate
}

func TestGetPodLifecycleState(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want appspub.LifecycleStateType
	}{
		{
			name: "label is PreparingNormal",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{appspub.LifecycleStateKey: "PreparingNormal"},
					},
				},
			},
			want: appspub.LifecycleStatePreparingNormal,
		},
		{
			name: "pod has unrelated labels only",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"unrelated": "value"},
					},
				},
			},
			want: "",
		},
		{
			name: "labels is nil",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: nil,
					},
				},
			},
			want: "",
		},
		{
			name: "pod is nil",
			args: args{
				pod: nil,
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPodLifecycleState(tt.args.pod); got != tt.want {
				t.Errorf("GetPodLifecycleState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsHookMarkPodNotReady(t *testing.T) {
	type args struct {
		lifecycleHook *appspub.LifecycleHook
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "hook is nil",
			args: args{
				lifecycleHook: nil,
			},
			want: false,
		},
		{
			name: "hook mark pod not ready",
			args: args{
				lifecycleHook: &appspub.LifecycleHook{
					MarkPodNotReady: true,
				},
			},
			want: true,
		},
		{
			name: "hook does not mark pod not ready",
			args: args{
				lifecycleHook: &appspub.LifecycleHook{
					MarkPodNotReady: false,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHookMarkPodNotReady(tt.args.lifecycleHook); got != tt.want {
				t.Errorf("IsHookMarkPodNotReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLifecycleMarkPodNotReady(t *testing.T) {
	type args struct {
		lifecycle *appspub.Lifecycle
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "lifecycle is nil",
			args: args{
				lifecycle: nil,
			},
			want: false,
		},
		{
			name: "both hooks are nil",
			args: args{
				lifecycle: &appspub.Lifecycle{
					PreDelete:     nil,
					InPlaceUpdate: nil,
				},
			},
			want: false,
		},
		{
			name: "PreDelete marks pod not ready",
			args: args{lifecycle: &appspub.Lifecycle{
				PreDelete: &appspub.LifecycleHook{MarkPodNotReady: true},
			}},
			want: true,
		},
		{
			name: "InPlaceUpdate marks pod not ready",
			args: args{lifecycle: &appspub.Lifecycle{
				InPlaceUpdate: &appspub.LifecycleHook{MarkPodNotReady: true},
			}},
			want: true,
		},
		{
			name: "both hooks are false",
			args: args{lifecycle: &appspub.Lifecycle{
				PreDelete:     &appspub.LifecycleHook{MarkPodNotReady: false},
				InPlaceUpdate: &appspub.LifecycleHook{MarkPodNotReady: false},
			}},
			want: false,
		},
		{
			name: "both hooks are true",
			args: args{lifecycle: &appspub.Lifecycle{
				PreDelete:     &appspub.LifecycleHook{MarkPodNotReady: true},
				InPlaceUpdate: &appspub.LifecycleHook{MarkPodNotReady: true},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLifecycleMarkPodNotReady(tt.args.lifecycle); got != tt.want {
				t.Errorf("IsLifecycleMarkPodNotReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetPodLifecycle(t *testing.T) {
	type args struct {
		state appspub.LifecycleStateType
		pod   *corev1.Pod
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "pod is nil",
			args: args{
				state: appspub.LifecycleStateNormal,
				pod:   nil,
			},
		},
		{
			name: "empty pod, set state to Normal",
			args: args{
				state: appspub.LifecycleStateNormal,
				pod:   &corev1.Pod{},
			},
		},
		{
			name: "overwrite existing lifecycle state",
			args: args{
				state: appspub.LifecycleStateNormal,
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)},
					},
				},
			},
		},
		{
			name: "annotation is nil, should initialize",
			args: args{
				state: appspub.LifecycleStateNormal,
				pod:   &corev1.Pod{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetPodLifecycle(tt.args.state)(tt.args.pod)
			if tt.args.pod == nil {
				return
			}
			if tt.args.pod.Labels[appspub.LifecycleStateKey] != string(tt.args.state) {
				t.Errorf("SetPodLifecycle() = %v, want %v", tt.args.pod.Labels[appspub.LifecycleStateKey], string(tt.args.state))
			}
			if tt.args.pod.Annotations == nil {
				t.Errorf("SetPodLifecycle() doesn't initialize annotations")
			}
			if ts := tt.args.pod.Annotations[appspub.LifecycleTimestampKey]; ts == "" {
				t.Errorf("SetPodLifecycle() missing timestamp annotation")
			}
		})
	}
}

func TestIsPodHooked(t *testing.T) {
	type args struct {
		hook *appspub.LifecycleHook
		pod  *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "hook is nil",
			args: args{
				hook: nil,
				pod:  &corev1.Pod{},
			},
			want: false,
		},
		{
			name: "pod is nil",
			args: args{
				hook: &appspub.LifecycleHook{},
				pod:  nil,
			},
			want: false,
		},
		{
			name: "finalizer matches",
			args: args{
				hook: &appspub.LifecycleHook{
					FinalizersHandler: []string{"kruise.io/unready-blocker"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"kruise.io/unready-blocker"},
					},
				},
			},
			want: true,
		},
		{
			name: "label matches",
			args: args{
				hook: &appspub.LifecycleHook{
					LabelsHandler: map[string]string{"kruise.io/unready-blocker": "true"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"kruise.io/unready-blocker": "true"},
					},
				},
			},
			want: true,
		},
		{
			name: "finalizer and label both match",
			args: args{
				hook: &appspub.LifecycleHook{
					FinalizersHandler: []string{"kruise.io/unready-blocker"},
					LabelsHandler:     map[string]string{"kruise.io/unready-blocker": "true"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"kruise.io/unready-blocker"},
						Labels:     map[string]string{"kruise.io/unready-blocker": "true"},
					},
				},
			},
			want: true,
		},
		{
			name: "no match",
			args: args{
				hook: &appspub.LifecycleHook{
					FinalizersHandler: []string{"kruise.io/unready-blocker"},
					LabelsHandler:     map[string]string{"kruise.io/unready-blocker": "true"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"kruise.io/unready-blocker-other"},
						Labels:     map[string]string{"kruise.io/unready-blocker-other": "true"},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPodHooked(tt.args.hook, tt.args.pod); got != tt.want {
				t.Errorf("IsPodHooked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPodAllHooked(t *testing.T) {
	type args struct {
		hook *appspub.LifecycleHook
		pod  *corev1.Pod
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "hook is nil",
			args: args{
				hook: nil,
				pod:  &corev1.Pod{},
			},
			want: false,
		},
		{
			name: "pod is nil",
			args: args{
				hook: &appspub.LifecycleHook{},
				pod:  nil,
			},
			want: false,
		},
		{
			name: "missing one finalizer",
			args: args{
				hook: &appspub.LifecycleHook{
					FinalizersHandler: []string{"kruise.io/unready-blocker-a", "kruise.io/unready-blocker-b"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"kruise.io/unready-blocker-a"},
					},
				},
			},
			want: false,
		},
		{
			name: "label value mismatch",
			args: args{
				hook: &appspub.LifecycleHook{
					LabelsHandler: map[string]string{"kruise.io/unready-blocker": "true"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"kruise.io/unready-blocker": "false"},
					},
				},
			},
			want: false,
		},
		{
			name: "finalizer and label both match completely",
			args: args{
				hook: &appspub.LifecycleHook{
					FinalizersHandler: []string{"kruise.io/unready-blocker-a", "kruise.io/unready-blocker-b"},
					LabelsHandler:     map[string]string{"kruise.io/unready-blocker": "true"},
				},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"kruise.io/unready-blocker-a", "kruise.io/unready-blocker-b"},
						Labels:     map[string]string{"kruise.io/unready-blocker": "true"},
					},
				},
			},
			want: true,
		},
		{
			name: "hook is empty, pod is non-empty",
			args: args{
				hook: &appspub.LifecycleHook{},
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{"kruise.io/unready-blocker"},
						Labels:     map[string]string{"kruise.io/unready-blocker": "true"},
					},
				},
			},
			want: true,
		},
		{
			name: "hook and pod are both empty",
			args: args{
				hook: &appspub.LifecycleHook{},
				pod:  &corev1.Pod{},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPodAllHooked(tt.args.hook, tt.args.pod); got != tt.want {
				t.Errorf("IsPodAllHooked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_realControl_executePodNotReadyPolicy(t *testing.T) {
	type args struct {
		state appspub.LifecycleStateType
	}
	tests := []struct {
		name       string
		args       args
		wantAdd    bool
		wantRemove bool
		wantErr    bool
	}{
		{
			name: "preparingDelete triggers AddNotReadyKey",
			args: args{
				state: appspub.LifecycleStatePreparingDelete,
			},
			wantAdd: true,
		},
		{
			name:    "preparingUpdate triggers AddNotReadyKey",
			args:    args{state: appspub.LifecycleStatePreparingUpdate},
			wantAdd: true,
		},
		{
			name:       "Updated triggers RemoveNotReadyKey",
			args:       args{appspub.LifecycleStateUpdated},
			wantRemove: true,
		},
		{
			name:       "Normal triggers nothing",
			args:       args{appspub.LifecycleStateNormal},
			wantAdd:    false,
			wantRemove: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakePodReadinessControl{}
			rc := &realControl{
				podReadinessControl: fake,
			}
			pod := &corev1.Pod{}
			err := rc.executePodNotReadyPolicy(pod, tt.args.state)
			if tt.wantErr && err == nil {
				t.Errorf("rc.executePodNotReadyPolicy() expected error")
			}
			if tt.wantAdd && !fake.addCalled {
				t.Errorf("AddNotReadyKey not called")
			}
			if tt.wantRemove && !fake.removeCalled {
				t.Errorf("RemoveNotReadyKey not called")
			}
			if !tt.wantAdd && fake.addCalled {
				t.Errorf("AddNotReadyKey called")
			}
			if !tt.wantRemove && fake.removeCalled {
				t.Errorf("RemoveNotReadyKey called")
			}
		})
	}
}

func Test_realControl_UpdatePodLifecycle(t *testing.T) {
	t.Run("error from executePodNotReadyPolicy", func(t *testing.T) {
		silenceKlogForTest(t)
		rc := &realControl{
			adp: &fakeAdapter{},
			podReadinessControl: &fakePodReadinessControl{
				err: errors.New("readiness error"),
			},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{},
		}}
		updated, gotPod, err := rc.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, true)
		if err == nil || updated {
			t.Errorf("expected error, got nil; updated=%v", updated)
		}
		if gotPod != nil {
			t.Errorf("gotPod should be nil on failure")
		}
	})

	t.Run("no-op if already in target state", func(t *testing.T) {
		rc := &realControl{
			adp:                 &fakeAdapter{},
			podReadinessControl: &fakePodReadinessControl{},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingDelete)},
		}}
		updated, gotPod, err := rc.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, false)
		if err != nil || updated == true {
			t.Errorf("expected no update, got updated=%v err=%v", updated, err)
		}
		if gotPod != pod {
			t.Errorf("expected returned pod == input pod")
		}
	})

	t.Run("patch path triggered", func(t *testing.T) {
		adp := &fakePatchAdapter{}
		rc := &realControl{
			adp:                 adp,
			podReadinessControl: &fakePodReadinessControl{},
		}
		pod := &corev1.Pod{}
		updated, gotPod, err := rc.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, false)
		if err != nil || !updated || !adp.patchCalled {
			t.Errorf("patch path failed: updated=%v err=%v patchCalled=%v", updated, err, adp.patchCalled)
		}
		if gotPod == nil {
			t.Errorf("expected gotPod to be non-nil")
		}
	})

	t.Run("fallback to UpdatePod if no PatchPod", func(t *testing.T) {
		adp := &fakeAdapter{}
		rc := &realControl{
			adp:                 adp,
			podReadinessControl: &fakePodReadinessControl{},
		}
		pod := &corev1.Pod{}
		updated, gotPod, err := rc.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, false)
		if err != nil || !updated || !adp.updateCalled {
			t.Errorf("fallback update path failed: updated=%v err=%v updateCalled=%v", updated, err, adp.updateCalled)
		}
		if gotPod == nil {
			t.Errorf("expected gotPod to be non-nil")
		}
	})
}

func Test_realControl_UpdatePodLifecycleWithHandler(t *testing.T) {
	t.Run("empty handler or empty pod", func(t *testing.T) {
		rc := &realControl{
			adp:                 &fakeAdapter{},
			podReadinessControl: &fakePodReadinessControl{},
		}
		handler := &appspub.LifecycleHook{}
		pod := &corev1.Pod{}
		updated, gotPod, err := rc.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingDelete, nil)
		if err != nil || updated || gotPod == nil {
			t.Errorf("expected error, got nil; updated=%v, pod=%v", updated, gotPod)
		}
		updated, gotPod, err = rc.UpdatePodLifecycleWithHandler(nil, "", handler)
		if err != nil || updated || gotPod != nil {
			t.Errorf("expected error, got nil; updated=%v, pod=%v", updated, gotPod)
		}
	})
	t.Run("error from executePodNotReadyPolicy", func(t *testing.T) {
		silenceKlogForTest(t)
		rc := &realControl{
			adp: &fakeAdapter{},
			podReadinessControl: &fakePodReadinessControl{
				err: errors.New("readiness error"),
			},
		}
		handler := &appspub.LifecycleHook{
			MarkPodNotReady: true,
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{},
		}}
		updated, gotPod, err := rc.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingDelete, handler)
		if err == nil || updated {
			t.Errorf("expected error, got nil; updated=%v", updated)
		}
		if gotPod != nil {
			t.Errorf("gotPod should be nil on failure")
		}
	})

	t.Run("no-op if already in target state", func(t *testing.T) {
		rc := &realControl{
			adp:                 &fakeAdapter{},
			podReadinessControl: &fakePodReadinessControl{},
		}
		handler := &appspub.LifecycleHook{
			MarkPodNotReady: false,
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingDelete)},
		}}
		updated, gotPod, err := rc.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingDelete, handler)
		if err != nil || updated == true {
			t.Errorf("expected no update, got updated=%v err=%v", updated, err)
		}
		if gotPod != pod {
			t.Errorf("expected returned pod == input pod")
		}
	})

	t.Run("patch path triggered", func(t *testing.T) {
		adp := &fakePatchAdapter{}
		rc := &realControl{
			adp:                 adp,
			podReadinessControl: &fakePodReadinessControl{},
		}
		handler := &appspub.LifecycleHook{
			MarkPodNotReady:   false,
			LabelsHandler:     map[string]string{"kruise.io/unready-blocker": "true"},
			FinalizersHandler: []string{"kruise.io/unready-blocker"},
		}
		pod := &corev1.Pod{}
		updated, gotPod, err := rc.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingDelete, handler)
		if err != nil || !updated || !adp.patchCalled {
			t.Errorf("patch path failed: updated=%v err=%v patchCalled=%v", updated, err, adp.patchCalled)
		}
		if gotPod == nil {
			t.Errorf("expected gotPod to be non-nil")
		}
	})

	t.Run("fallback to UpdatePod if no PatchPod", func(t *testing.T) {
		adp := &fakeAdapter{}
		rc := &realControl{
			adp:                 adp,
			podReadinessControl: &fakePodReadinessControl{},
		}
		handler := &appspub.LifecycleHook{
			MarkPodNotReady:   false,
			LabelsHandler:     map[string]string{"kruise.io/unready-blocker": "true"},
			FinalizersHandler: []string{"kruise.io/unready-blocker"},
		}
		pod := &corev1.Pod{}
		updated, gotPod, err := rc.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingDelete, handler)
		if err != nil || !updated || !adp.updateCalled {
			t.Errorf("fallback update path failed: updated=%v err=%v updateCalled=%v", updated, err, adp.updateCalled)
		}
		if gotPod == nil {
			t.Errorf("expected gotPod to be non-nil")
		}
	})
}

func silenceKlogForTest(t *testing.T) {
	originalStderr := os.Stderr
	nullFile, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("failed to open /dev/null: %v", err)
	}
	os.Stderr = nullFile

	t.Cleanup(func() {
		os.Stderr = originalStderr
	})
}
