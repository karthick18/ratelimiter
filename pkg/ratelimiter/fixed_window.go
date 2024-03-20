package ratelimiter

import (
	"sync"
	"sync/atomic"
	"time"
)

type fixedWindowCache struct {
	lock                   sync.RWMutex
	cache                  sync.Map
	requestInterval        time.Duration
	maxRequestsPerInterval int
	stop                   chan struct{}
}

var _ Cache = &fixedWindowCache{}

func newFixedWindowCache(requestInterval time.Duration, maxRequestsPerInterval int) *fixedWindowCache {
	cache := &fixedWindowCache{
		requestInterval:        requestInterval,
		maxRequestsPerInterval: maxRequestsPerInterval,
		stop:                   make(chan struct{}),
	}
	go cache.fixedWindowInterval()
	return cache
}

func (c *fixedWindowCache) Add(key interface{}) bool {
	limit := c.maxRequestsPerInterval
	c.lock.RLock()
	defer c.lock.RUnlock()
	status := false
	for !status {
		val, loaded := c.cache.LoadOrStore(key, new(int64))
		counter := val.(*int64)
		if !loaded {
			atomic.AddInt64(counter, 1)
			return true
		}
		oldValue := *counter
		if limit > 0 && oldValue >= int64(limit) {
			return false
		}
		newValue := oldValue + 1
		status = atomic.CompareAndSwapInt64(counter, oldValue, newValue)
		// if there is a failure in compare and swap, we raced with another thread
		// retry the compare and swap until we hit the limit
		if status {
			break
		}
	}

	return true
}

func (c *fixedWindowCache) Invalidate() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.cache = sync.Map{}
}

func (c *fixedWindowCache) Shutdown() {
	c.stop <- struct{}{}
	c.Invalidate()
}

func (c *fixedWindowCache) fixedWindowInterval() {
	ticker := time.NewTicker(c.requestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.Invalidate()
		case <-c.stop:
			return
		}
	}
}
