package ratelimiter

import (
	"testing"
	"time"
)

func TestDefaultControllerRateLimiter(t *testing.T) {
	t.Run("ExponentialBackoffAndMaxDelay", func(t *testing.T) {
		// Setup specific values for this test case
		baseDelay = 10 * time.Millisecond
		maxDelay = 100 * time.Millisecond
		qps = 50 
		bucketSize = 50

		limiter := DefaultControllerRateLimiter[int]()
		if limiter == nil {
			t.Fatal("DefaultControllerRateLimiter returned nil, want non-nil")
		}

		item := 123

		//1st attempt
		delay := limiter.When(item)
		if delay != baseDelay {
			t.Errorf("got delay %v, want %v", delay, baseDelay)
		}
		if numRequeues := limiter.NumRequeues(item); numRequeues != 1 {
			t.Errorf("got %d requeues, want 1", numRequeues)
		}

		// 2nd attempt
		delay = limiter.When(item)
		expectedDelay := 2 * baseDelay
		if delay != expectedDelay {
			t.Errorf("got delay %v, want %v", delay, expectedDelay)
		}
		if numRequeues := limiter.NumRequeues(item); numRequeues != 2 {
			t.Errorf("got %d requeues, want 2", numRequeues)
		}

		// 3rd attempt
		delay = limiter.When(item)
		expectedDelay = 4 * baseDelay
		if delay != expectedDelay {
			t.Errorf("got delay %v, want %v", delay, expectedDelay)
		}

		//subsequent attempts should eventually be capped by maxDelay.
		// 10*1=10, 10*2=20, 10*4=40, 10*8=80, 10*16=160 (capped at 100)
		limiter.When(item)
		delay = limiter.When(item)
		if delay != maxDelay {
			t.Errorf("got delay %v, want capped maxDelay %v", delay, maxDelay)
		}

		limiter.Forget(item)
		if numRequeues := limiter.NumRequeues(item); numRequeues != 0 {
			t.Errorf("got %d requeues after Forget, want 0", numRequeues)
		}

		delay = limiter.When(item)
		if delay != baseDelay {
			t.Errorf("got delay %v after Forget, want %v", delay, baseDelay)
		}
	})

	t.Run("TokenBucketRateLimiting", func(t *testing.T) {
		// Setup values to specifically test the token bucket
		baseDelay = 1 * time.Millisecond
		maxDelay = 5 * time.Second
		qps = 10
		bucketSize = 5

		limiter := DefaultControllerRateLimiter[string]()
		item := "test-item"

		for i := 0; i < bucketSize; i++ {
			limiter.When(item)
		}
		delay := limiter.When(item)
		expectedMinDelay := time.Second / time.Duration(qps)

		if delay < expectedMinDelay-(10*time.Millisecond) {
			t.Errorf("got delay %v, want at least %v due to token bucket rate limiting", delay, expectedMinDelay)
		}
	})
}
