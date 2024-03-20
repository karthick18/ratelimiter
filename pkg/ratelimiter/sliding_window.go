package ratelimiter

import (
	"container/heap"
	"sync"
	"time"
)

type slidingWindowCache struct {
	sync.Mutex
	requestInterval        time.Duration
	maxRequestsPerInterval int
	runningCounter         uint64
	cache                  map[interface{}]map[uint64]*cacheExpiryEntry
	stop                   chan struct{}
	input                  chan *cacheExpiryEntry
}

type cacheExpiryEntry struct {
	key       interface{}
	expiresAt time.Time
	// index of the data expiring for the key
	dataIndex uint64
	// index in the heap
	index int
}

var _ Cache = &slidingWindowCache{}

func newSlidingWindowCache(requestInterval time.Duration, maxRequestsPerInterval int) *slidingWindowCache {
	cache := &slidingWindowCache{
		cache:                  make(map[interface{}]map[uint64]*cacheExpiryEntry),
		requestInterval:        requestInterval,
		maxRequestsPerInterval: maxRequestsPerInterval,
		stop:                   make(chan struct{}),
		input:                  make(chan *cacheExpiryEntry, 3000),
	}
	go cache.processExpiry()

	return cache
}

func (c *slidingWindowCache) cacheAddWithLock(key interface{}, data *cacheExpiryEntry) {
	_, ok := c.cache[key]
	if !ok {
		c.cache[key] = make(map[uint64]*cacheExpiryEntry)
	}
	index := c.runningCounter
	data.dataIndex = index
	c.cache[key][index] = data
	c.runningCounter++
}

func (c *slidingWindowCache) cacheDelete(key interface{}, data ...*cacheExpiryEntry) {
	c.Lock()
	defer c.Unlock()
	if len(c.cache) == 0 {
		return
	}
	if len(data) == 0 {
		delete(c.cache, key)
	} else {
		delete(c.cache[key], data[0].dataIndex)
		if len(c.cache[key]) == 0 {
			delete(c.cache, key)
		}
	}
}

func (c *slidingWindowCache) cacheShutdown() {
	c.Lock()
	defer c.Unlock()
	c.cache = make(map[interface{}]map[uint64]*cacheExpiryEntry)
}

func (c *slidingWindowCache) Add(key interface{}) bool {
	limit := c.maxRequestsPerInterval
	// check the data limit on the key before adding the entry
	c.Lock()
	defer c.Unlock()
	entry, exists := c.cache[key]
	if limit > 0 && exists && len(entry) >= limit {
		return false
	}
	data := &cacheExpiryEntry{key: key, expiresAt: time.Now().Add(c.requestInterval)}
	c.cacheAddWithLock(key, data)
	c.input <- data

	return true
}

func (c *slidingWindowCache) Shutdown() {
	c.stop <- struct{}{}
	c.cacheShutdown()
}

type expiryPriorityQueue []*cacheExpiryEntry

var _ heap.Interface = &expiryPriorityQueue{}

func (pq expiryPriorityQueue) Len() int {
	return len(pq)
}

func (pq expiryPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq expiryPriorityQueue) Less(i, j int) bool {
	return pq[i].expiresAt.Before(pq[j].expiresAt)
}

func (pq *expiryPriorityQueue) Push(entry interface{}) {
	index := len(*pq)
	e := entry.(*cacheExpiryEntry)
	e.index = index
	*pq = append(*pq, e)
}

func (pq *expiryPriorityQueue) Pop() interface{} {
	l := len(*pq)
	entry := (*pq)[l-1]
	entry.index = -1
	*pq = (*pq)[:l-1]

	return entry
}

func (pq expiryPriorityQueue) Peek() interface{} {
	return pq[0]
}

func (c *slidingWindowCache) processExpiry() {
	pq := &expiryPriorityQueue{}
	heap.Init(pq)
	nextExpiry := make(<-chan time.Time)
	var nextExpiryC <-chan time.Time

	for {
		t := time.Now()
		for pq.Len() > 0 {
			nextExpiryC = nextExpiry
			top := pq.Peek().(*cacheExpiryEntry)
			if top.expiresAt.After(t) {
				nextExpiryC = time.NewTimer(top.expiresAt.Sub(t)).C
				break
			}
			// cache entry has expired. pop the entry and remove from cache as appropriate
			top = pq.Pop().(*cacheExpiryEntry)
			c.cacheDelete(top.key, top)
		}

		select {
		case <-c.stop:
			return
		case <-nextExpiryC:
			// it will process the cache expiry
		case expiryEntry := <-c.input:
			// insert only if it's a per client sliding window
			// for fixed windows, we can just reset the cache entries every fixed interval
			c.insert(pq, expiryEntry)
		}
	}
}

func (c *slidingWindowCache) insert(pq *expiryPriorityQueue, expiryEntry *cacheExpiryEntry) {
	heap.Push(pq, expiryEntry)
}
