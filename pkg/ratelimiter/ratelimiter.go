package ratelimiter

import (
	"time"
)

type Interface interface {
	ShouldAllow(string) bool
}

func New(requestInterval time.Duration, maxRequestsPerInterval int) Interface {
	return newRateLimiter(requestInterval, maxRequestsPerInterval)
}

type rateLimiterImpl struct {
	cache                  *expiryCache
	requestInterval        time.Duration
	maxRequestsPerInterval int
}

func newRateLimiter(requestInterval time.Duration, maxRequestsPerInterval int) *rateLimiterImpl {
	return &rateLimiterImpl{
		cache:                  newExpiryCache(requestInterval),
		requestInterval:        requestInterval,
		maxRequestsPerInterval: maxRequestsPerInterval,
	}
}

func (r *rateLimiterImpl) ShouldAllow(key string) bool {
	return r.cache.AddWithLimit(key, r.maxRequestsPerInterval)
}
