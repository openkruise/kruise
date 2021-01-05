/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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
	"sync"
	"testing"
)

func TestSlowStartBatch(t *testing.T) {
	fakeErr := fmt.Errorf("fake error")
	callCnt := 0
	callLimit := 0
	var lock sync.Mutex
	fn := func(idx int) error {
		lock.Lock()
		defer lock.Unlock()
		callCnt++
		if callCnt > callLimit {
			return fakeErr
		}
		return nil
	}

	tests := []struct {
		name              string
		count             int
		callLimit         int
		fn                func(int) error
		expectedSuccesses int
		expectedErr       error
		expectedCallCnt   int
	}{
		{
			name:              "callLimit = 0 (all fail)",
			count:             10,
			callLimit:         0,
			fn:                fn,
			expectedSuccesses: 0,
			expectedErr:       fakeErr,
			expectedCallCnt:   1, // 1(first batch): function will be called at least once
		},
		{
			name:              "callLimit = count (all succeed)",
			count:             10,
			callLimit:         10,
			fn:                fn,
			expectedSuccesses: 10,
			expectedErr:       nil,
			expectedCallCnt:   10, // 1(first batch) + 2(2nd batch) + 4(3rd batch) + 3(4th batch) = 10
		},
		{
			name:              "callLimit < count (some succeed)",
			count:             10,
			callLimit:         5,
			fn:                fn,
			expectedSuccesses: 5,
			expectedErr:       fakeErr,
			expectedCallCnt:   7, // 1(first batch) + 2(2nd batch) + 4(3rd batch) = 7
		},
	}

	for _, test := range tests {
		callCnt = 0
		callLimit = test.callLimit
		successes, err := SlowStartBatch(test.count, 1, test.fn)
		if successes != test.expectedSuccesses {
			t.Errorf("%s: unexpected processed batch size, expected %d, got %d", test.name, test.expectedSuccesses, successes)
		}
		if err != test.expectedErr {
			t.Errorf("%s: unexpected processed batch size, expected %v, got %v", test.name, test.expectedErr, err)
		}
		// verify that slowStartBatch stops trying more calls after a batch fails
		if callCnt != test.expectedCallCnt {
			t.Errorf("%s: slowStartBatch() still tries calls after a batch fails, expected %d calls, got %d", test.name, test.expectedCallCnt, callCnt)
		}
	}
}

func TestIsContainerImageEqual(t *testing.T) {
	cases := []struct {
		name       string
		images     [2]string
		equal      bool
		exceptions map[string][3]string
	}{
		{
			name:   "image tag and equal",
			images: [2]string{"docker.io/busybox:v1", "docker.io/busybox:v1"},
			equal:  true,
			exceptions: map[string][3]string{
				"docker.io/busybox:v1": {"docker.io/busybox", "v1", ""},
			},
		},
		{
			name:   "image tag and not equal",
			images: [2]string{"docker.io/busybox:v1", "docker.io/busybox:v2"},
			equal:  false,
			exceptions: map[string][3]string{
				"docker.io/busybox:v1": {"docker.io/busybox", "v1", ""},
				"docker.io/busybox:v2": {"docker.io/busybox", "v2", ""},
			},
		},
		{
			name:   "image digest and equal",
			images: [2]string{"docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d", "docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"},
			equal:  true,
			exceptions: map[string][3]string{
				"docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d": {"docker.io/busybox", "", "sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"},
			},
		},
		{
			name:   "image digest and not equal",
			images: [2]string{"docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d", "docker.io/busybox@sha256:a2d86defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"},
			equal:  false,
			exceptions: map[string][3]string{
				"docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d": {"docker.io/busybox", "", "sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"},
				"docker.io/busybox@sha256:a2d86defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d": {"docker.io/busybox", "", "sha256:a2d86defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"},
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			if cs.equal != IsContainerImageEqual(cs.images[0], cs.images[1]) {
				t.Fatalf("except %t, but get %t", cs.equal, IsContainerImageEqual(cs.images[0], cs.images[1]))
			}
			for image, excepts := range cs.exceptions {
				repo, tag, digest, err := ParseImage(image)
				if err != nil {
					t.Errorf("ParseImage %s failed: %s", image, err.Error())
				}
				if repo != excepts[0] || tag != excepts[1] || digest != excepts[2] {
					t.Fatalf("except repo %s tag %s digest %s, but get %s, %s, %s",
						excepts[0], excepts[1], excepts[2], repo, tag, digest)
				}
			}
		})
	}
}
