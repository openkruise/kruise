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
	fn := func() error {
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
		fn                func() error
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
