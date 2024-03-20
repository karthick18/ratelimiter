package ratelimiter

type Cache interface {
	Add(interface{}) bool
	Shutdown()
}
