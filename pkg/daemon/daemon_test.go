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

package daemon

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestErrSignaler(t *testing.T) {
	t.Run("SignalError stores first error and ignores subsequent ones", func(t *testing.T) {
		s := &errSignaler{errSignal: make(chan struct{})}
		firstErr := errors.New("this is the first error")
		secondErr := errors.New("this error should be ignored")

		// Signal the first error
		s.SignalError(firstErr)
		s.SignalError(secondErr)

		// Check that the channel is closed
		select {
		case <-s.GotError():
		case <-time.After(1 * time.Second):
			t.Fatal("errSignal channel was not closed after an error was signaled")
		}

		// Check that only the first error was stored
		assert.Equal(t, firstErr, s.Error(), "Should only store the first error")
	})

	t.Run("SignalError with nil should be ignored", func(t *testing.T) {
		s := &errSignaler{errSignal: make(chan struct{})}

		s.SignalError(nil)

		// Check that the channel remains open
		select {
		case <-s.GotError():
			t.Fatal("errSignal channel should not be closed for a nil error")
		default:
		}

		assert.NoError(t, s.Error())
	})

	t.Run("Concurrent SignalError calls are handled safely", func(t *testing.T) {
		s := &errSignaler{errSignal: make(chan struct{})}
		err1 := errors.New("error 1")
		err2 := errors.New("error 2")

		var wg sync.WaitGroup
		wg.Add(2)

		// Signal two errors concurrently
		go func() {
			defer wg.Done()
			s.SignalError(err1)
		}()
		go func() {
			defer wg.Done()
			s.SignalError(err2)
		}()

		wg.Wait()

		storedErr := s.Error()
		assert.NotNil(t, storedErr)
		assert.True(t, storedErr == err1 || storedErr == err2, "Stored error should be one of the signaled errors")

		select {
		case <-s.GotError():
		default:
			t.Fatal("errSignal channel was not closed after concurrent signals")
		}
	})
}
