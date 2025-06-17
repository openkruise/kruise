package imagepuller

import (
	"sync"
	"testing"
	"time"
)

func TestChanPool_Submit_Start_Stop(t *testing.T) {
	taskCount := 10
	poolSize := 3
	pool := NewChanPool(poolSize)
	pool.Start()
	var wg sync.WaitGroup
	wg.Add(taskCount)

	executionCount := 0
	mu := sync.Mutex{}

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			executionCount++
			mu.Unlock()
		})
	}

	wg.Wait()
	pool.Stop()

	if executionCount != taskCount {
		t.Errorf("expected %d tasks to be executed, but got %d", taskCount, executionCount)
	}
}

func TestChanPool_StopWithoutTasks(t *testing.T) {
	pool := NewChanPool(2)
	pool.Start()
	pool.Stop()

	select {
	case <-time.After(100 * time.Millisecond):
		t.Errorf("expected Stop to complete immediately, but it took too long")
	default:

	}
}
