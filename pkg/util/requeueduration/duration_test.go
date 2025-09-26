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

package requeueduration

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDurationStore_PopFromEmpty(t *testing.T) {
	store := &DurationStore{}

	duration := store.Pop("non-existent-key")
	assert.Equal(t, time.Duration(0), duration)
}

func TestDurationStore_PushAndPop(t *testing.T) {
	store := &DurationStore{}
	key := "test-key"
	expectedDuration := 5 * time.Second

	// Push duration
	store.Push(key, expectedDuration)

	// Pop should return the duration and remove the key
	duration := store.Pop(key)
	assert.Equal(t, expectedDuration, duration)

	// Second pop should return 0 since key was removed
	duration = store.Pop(key)
	assert.Equal(t, time.Duration(0), duration)
}

func TestDurationStore_MultipleKeys(t *testing.T) {
	store := &DurationStore{}

	// Push different durations for different keys
	store.Push("key1", 1*time.Second)
	store.Push("key2", 2*time.Second)
	store.Push("key3", 3*time.Second)

	// Pop in different order
	assert.Equal(t, 2*time.Second, store.Pop("key2"))
	assert.Equal(t, 1*time.Second, store.Pop("key1"))
	assert.Equal(t, 3*time.Second, store.Pop("key3"))

	// All keys should be removed
	assert.Equal(t, time.Duration(0), store.Pop("key1"))
	assert.Equal(t, time.Duration(0), store.Pop("key2"))
	assert.Equal(t, time.Duration(0), store.Pop("key3"))
}

func TestDurationStore_PushMultipleTimes(t *testing.T) {
	store := &DurationStore{}
	key := "test-key"

	// Push multiple durations - should keep the shortest
	store.Push(key, 10*time.Second)
	store.Push(key, 5*time.Second)
	store.Push(key, 15*time.Second)

	duration := store.Pop(key)
	assert.Equal(t, 5*time.Second, duration)
}

func TestDurationStore_ConcurrentAccess(t *testing.T) {
	store := &DurationStore{}
	key := "concurrent-key"
	numGoroutines := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent pushes
	for i := 0; i < numGoroutines; i++ {
		go func(duration time.Duration) {
			defer wg.Done()
			store.Push(key, duration)
		}(time.Duration(i+1) * time.Millisecond)
	}

	wg.Wait()

	// Should get the shortest duration (1ms)
	duration := store.Pop(key)
	assert.Equal(t, 1*time.Millisecond, duration)
}

func TestDuration_Update(t *testing.T) {
	tests := []struct {
		name            string
		initialDuration time.Duration
		newDuration     time.Duration
		expectedResult  time.Duration
	}{
		{
			name:            "update from zero to positive",
			initialDuration: 0,
			newDuration:     5 * time.Second,
			expectedResult:  5 * time.Second,
		},
		{
			name:            "update to shorter duration",
			initialDuration: 10 * time.Second,
			newDuration:     3 * time.Second,
			expectedResult:  3 * time.Second,
		},
		{
			name:            "ignore longer duration",
			initialDuration: 5 * time.Second,
			newDuration:     10 * time.Second,
			expectedResult:  5 * time.Second,
		},
		{
			name:            "ignore zero duration",
			initialDuration: 5 * time.Second,
			newDuration:     0,
			expectedResult:  5 * time.Second,
		},
		{
			name:            "ignore negative duration",
			initialDuration: 5 * time.Second,
			newDuration:     -1 * time.Second,
			expectedResult:  5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := &Duration{duration: tt.initialDuration}
			duration.Update(tt.newDuration)
			assert.Equal(t, tt.expectedResult, duration.Get())
		})
	}
}

func TestDuration_UpdateWithMsg(t *testing.T) {
	duration := &Duration{}

	// First update with message
	duration.UpdateWithMsg(5*time.Second, "waiting for %s", "pod")
	resultDuration, message := duration.GetWithMsg()
	assert.Equal(t, 5*time.Second, resultDuration)
	assert.Equal(t, "waiting for pod", message)

	// Update with shorter duration and new message
	duration.UpdateWithMsg(3*time.Second, "waiting for %s in %s", "container", "namespace")
	resultDuration, message = duration.GetWithMsg()
	assert.Equal(t, 3*time.Second, resultDuration)
	assert.Equal(t, "waiting for container in namespace", message)

	// Update with longer duration - should not change
	duration.UpdateWithMsg(10*time.Second, "should not update")
	resultDuration, message = duration.GetWithMsg()
	assert.Equal(t, 3*time.Second, resultDuration)
	assert.Equal(t, "waiting for container in namespace", message)
}

func TestDuration_Merge(t *testing.T) {
	duration1 := &Duration{duration: 10 * time.Second, message: "original message"}
	duration2 := &Duration{duration: 5 * time.Second, message: "shorter message"}

	// Merge duration2 into duration1
	duration1.Merge(duration2)

	resultDuration, message := duration1.GetWithMsg()
	assert.Equal(t, 5*time.Second, resultDuration)
	assert.Equal(t, "shorter message", message)
}

func TestDuration_ConcurrentOperations(t *testing.T) {
	duration := &Duration{}
	numGoroutines := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent updates
	for i := 0; i < numGoroutines; i++ {
		go func(d time.Duration) {
			defer wg.Done()
			duration.Update(d)
		}(time.Duration(i+1) * time.Millisecond)
	}

	// Concurrent gets
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = duration.Get()
		}()
	}

	wg.Wait()

	// Should have the shortest duration (1ms)
	assert.Equal(t, 1*time.Millisecond, duration.Get())
}

func TestDurationStore_InvalidTypeHandling(t *testing.T) {
	store := &DurationStore{}
	key := "test-key"

	// Manually store an invalid type to test type assertion handling
	store.store.Store(key, "invalid-type")

	// Push should handle invalid type gracefully by deleting the key
	store.Push(key, 5*time.Second)

	// Pop should return 0 for invalid type
	duration := store.Pop(key)
	assert.Equal(t, time.Duration(0), duration)
}

func TestDuration_ZeroInitialization(t *testing.T) {
	duration := &Duration{}

	// Initial values should be zero
	assert.Equal(t, time.Duration(0), duration.Get())

	resultDuration, message := duration.GetWithMsg()
	assert.Equal(t, time.Duration(0), resultDuration)
	assert.Equal(t, "", message)
}

func TestDuration_MessageFormatting(t *testing.T) {
	// Test various message formatting scenarios
	tests := []struct {
		name        string
		format      string
		args        []interface{}
		expectedMsg string
	}{
		{
			name:        "simple string",
			format:      "simple message",
			args:        nil,
			expectedMsg: "simple message",
		},
		{
			name:        "single argument",
			format:      "waiting for %s",
			args:        []interface{}{"pod"},
			expectedMsg: "waiting for pod",
		},
		{
			name:        "multiple arguments",
			format:      "waiting for %s in namespace %s for %d seconds",
			args:        []interface{}{"pod", "default", 30},
			expectedMsg: "waiting for pod in namespace default for 30 seconds",
		},
		{
			name:        "empty format",
			format:      "",
			args:        nil,
			expectedMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new Duration for each test case
			duration := &Duration{}
			duration.UpdateWithMsg(1*time.Second, tt.format, tt.args...)
			_, message := duration.GetWithMsg()
			assert.Equal(t, tt.expectedMsg, message)
		})
	}
}

func TestDuration_MergeWithEmptyDuration(t *testing.T) {
	duration1 := &Duration{duration: 5 * time.Second, message: "original"}
	duration2 := &Duration{}

	// Merge empty duration - should not change duration1
	duration1.Merge(duration2)

	resultDuration, message := duration1.GetWithMsg()
	assert.Equal(t, 5*time.Second, resultDuration)
	assert.Equal(t, "original", message)
}

func TestDuration_MergeIntoEmptyDuration(t *testing.T) {
	duration1 := &Duration{}
	duration2 := &Duration{duration: 5 * time.Second, message: "new message"}

	// Merge into empty duration
	duration1.Merge(duration2)

	resultDuration, message := duration1.GetWithMsg()
	assert.Equal(t, 5*time.Second, resultDuration)
	assert.Equal(t, "new message", message)
}

func TestDurationStore_EdgeCases(t *testing.T) {
	store := &DurationStore{}

	// Test with empty key
	store.Push("", 5*time.Second)
	duration := store.Pop("")
	assert.Equal(t, 5*time.Second, duration)

	// Test with very long key
	longKey := string(make([]byte, 1000))
	store.Push(longKey, 3*time.Second)
	duration = store.Pop(longKey)
	assert.Equal(t, 3*time.Second, duration)

	// Test with special characters in key
	specialKey := "key/with:special@characters#"
	store.Push(specialKey, 2*time.Second)
	duration = store.Pop(specialKey)
	assert.Equal(t, 2*time.Second, duration)
}

func TestDuration_ExtremeValues(t *testing.T) {
	duration := &Duration{}

	// Test with very large duration
	largeDuration := time.Duration(1<<62 - 1)
	duration.Update(largeDuration)
	assert.Equal(t, largeDuration, duration.Get())

	// Test with very small positive duration
	smallDuration := 1 * time.Nanosecond
	duration.Update(smallDuration)
	assert.Equal(t, smallDuration, duration.Get())

	// Test with maximum negative duration (should be ignored)
	duration.Update(-1 << 62)
	assert.Equal(t, smallDuration, duration.Get())
}

func TestDurationStore_ConcurrentPushPop(t *testing.T) {
	store := &DurationStore{}
	key := "concurrent-test"
	numOperations := 50

	var wg sync.WaitGroup
	results := make([]time.Duration, numOperations)

	// Start concurrent push and pop operations
	for i := 0; i < numOperations; i++ {
		wg.Add(2)

		go func(index int) {
			defer wg.Done()
			store.Push(key, time.Duration(index+1)*time.Millisecond)
		}(i)

		go func(index int) {
			defer wg.Done()
			results[index] = store.Pop(key)
		}(i)
	}

	wg.Wait()

	// Count non-zero results (successful pops)
	nonZeroCount := 0
	for _, result := range results {
		if result > 0 {
			nonZeroCount++
		}
	}

	assert.True(t, nonZeroCount > 0, "Should have at least some successful pops")
	assert.True(t, nonZeroCount <= numOperations, "Should not have more pops than pushes")
}
