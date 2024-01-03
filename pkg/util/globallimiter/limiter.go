package globallimiter

import (
	"sync"
	"time"
)

// ParallelismLimiter is designed to limit
type ParallelismLimiter interface {
	// Add return true if add key successfully, which means the number
	// of keys recorded in this limiter is less than its parallelism limitation.
	Add(key string) bool
	// Del return true if delete key successfully, which means this limiter
	// release a key from parallelism limitation.
	Del(key string) bool
}

// NewParallelismLimiter return a ParallelismLimiter which help you control parallelism about different keys.
func NewParallelismLimiter(limit int, activeKeys map[string]time.Time, timeout time.Duration) ParallelismLimiter {
	if activeKeys == nil {
		activeKeys = map[string]time.Time{}
	}
	limiter := &realGlobalLimiter{
		limit:      limit,
		activeKeys: activeKeys,
	}
	go limiter.gcDeadKey(timeout)
	return limiter
}

type realGlobalLimiter struct {
	limit      int
	lock       sync.Mutex
	activeKeys map[string]time.Time
}

func (g *realGlobalLimiter) Del(key string) bool {
	g.lock.Lock()
	defer g.lock.Unlock()
	delete(g.activeKeys, key)
	return true
}

func (g *realGlobalLimiter) Add(key string) bool {
	g.lock.Lock()
	defer g.lock.Unlock()
	if _, ok := g.activeKeys[key]; ok {
		g.activeKeys[key] = time.Now()
		return true
	}
	if len(g.activeKeys) < g.limit {
		g.activeKeys[key] = time.Now()
		return true
	}
	return false
}

// gcDeadKey is designed to avoid some bad cases:
// - the object corresponding to the key is deleted, but not use Dec func to release the key;
// - the progressing duration to some key is too long to tolerate;
func (g *realGlobalLimiter) gcDeadKey(timeout time.Duration) {
	gcFunc := func() {
		g.lock.Lock()
		defer g.lock.Unlock()
		ttl := time.Now().Add(-timeout)

		// TODO: optimize the data structure
		for key, lastActiveTime := range g.activeKeys {
			if lastActiveTime.Before(ttl) {
				delete(g.activeKeys, key)
			}
		}
	}

	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			gcFunc()
		}
	}
}
