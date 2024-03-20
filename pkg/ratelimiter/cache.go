package ratelimiter

import (
	"container/heap"
	"sync"
	"time"
)

type expiryCache struct {
	sync.Mutex
	runningCounter uint64
	cache          map[interface{}]map[uint64]*cacheExpiryEntry
	expiry         time.Duration
	stop           chan struct{}
	input          chan *cacheExpiryEntry
}

type cacheExpiryEntry struct {
	key       interface{}
	expiresAt time.Time
	// index of the data expiring for the key
	dataIndex uint64
	// index in the heap
	index int
}

func newExpiryCache(expiry time.Duration) *expiryCache {
	expCache := &expiryCache{
		cache:  make(map[interface{}]map[uint64]*cacheExpiryEntry),
		expiry: expiry,
		stop:   make(chan struct{}),
		input:  make(chan *cacheExpiryEntry, 3000),
	}
	go expCache.processExpiry()

	return expCache
}

func (c *expiryCache) cacheAddWithLock(key string, data *cacheExpiryEntry) {
	_, ok := c.cache[key]
	if !ok {
		c.cache[key] = make(map[uint64]*cacheExpiryEntry)
	}
	index := c.runningCounter
	data.dataIndex = index
	c.cache[key][index] = data
	c.runningCounter++
}

func (c *expiryCache) cacheAdd(key string, data *cacheExpiryEntry) {
	c.Lock()
	defer c.Unlock()

	c.cacheAddWithLock(key, data)
}

func (c *expiryCache) cacheGet(key interface{}) (map[uint64]*cacheExpiryEntry, bool) {
	c.Lock()
	defer c.Unlock()
	entry, exists := c.cache[key]
	if !exists {
		return nil, false
	}
	return entry, true
}

func (c *expiryCache) cacheGetEntries(key interface{}) int {
	c.Lock()
	defer c.Unlock()
	entry, exists := c.cache[key]
	if !exists {
		return 0
	}

	return len(entry)
}

func (c *expiryCache) cacheDelete(key interface{}, data ...*cacheExpiryEntry) {
	c.Lock()
	defer c.Unlock()
	if len(data) == 0 {
		delete(c.cache, key)
	} else {
		delete(c.cache[key], data[0].dataIndex)
		if len(c.cache[key]) == 0 {
			delete(c.cache, key)
		}
	}
}

func (c *expiryCache) cacheShutdown() {
	c.Lock()
	defer c.Unlock()

	for k := range c.cache {
		delete(c.cache, k)
	}
}

func (ec *expiryCache) Add(key string) {
	data := &cacheExpiryEntry{key: key, expiresAt: time.Now().Add(ec.expiry)}
	// add to cache and update dataIndex
	ec.cacheAdd(key, data)
	ec.input <- data
}

func (ec *expiryCache) AddWithLimit(key string, limit int) bool {
	// check the data limit on the key before adding the entry
	ec.Lock()
	defer ec.Unlock()
	entry, exists := ec.cache[key]
	if exists && len(entry) >= limit {
		return false
	}
	data := &cacheExpiryEntry{key: key, expiresAt: time.Now().Add(ec.expiry)}
	ec.cacheAddWithLock(key, data)
	ec.input <- data

	return true
}

func (ec *expiryCache) Get(key string) int {
	// returns the number of data entries in the cache
	return ec.cacheGetEntries(key)
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

func (ec *expiryCache) processExpiry() {
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
			ec.cacheDelete(top.key, top)
		}

		select {
		case <-ec.stop:
			return
		case <-nextExpiryC:
			// it will process the cache expiry
		case expiryEntry := <-ec.input:
			ec.insert(pq, expiryEntry)
		}
	}
}

func (ec *expiryCache) insert(pq *expiryPriorityQueue, expiryEntry *cacheExpiryEntry) {
	heap.Push(pq, expiryEntry)
}

func (ec *expiryCache) Shutdown() {
	ec.cacheShutdown()
	ec.stop <- struct{}{}
}
