package ratelimiter

import (
	"time"
)

type Interface interface {
	ShouldAllow(string) bool
}

func New(requestInterval time.Duration, maxRequestsPerInterval int, perClientSlidingWindow ...bool) Interface {
	return newRateLimiter(requestInterval, maxRequestsPerInterval, perClientSlidingWindow...)
}

type rateLimiterImpl struct {
	cache Cache
}

func newRateLimiter(requestInterval time.Duration, maxRequestsPerInterval int,
	perClientSlidingWindow ...bool) *rateLimiterImpl {
	slidingWindow := false
	if len(perClientSlidingWindow) > 0 {
		slidingWindow = perClientSlidingWindow[0]
	}
	var cache Cache
	if slidingWindow {
		cache = newSlidingWindowCache(requestInterval, maxRequestsPerInterval)
	} else {
		cache = newFixedWindowCache(requestInterval, maxRequestsPerInterval)
	}
	return &rateLimiterImpl{
		cache: cache,
	}
}

func (r *rateLimiterImpl) ShouldAllow(key string) bool {
	return r.cache.Add(key)
}
