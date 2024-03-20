package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/karthick18/ratelimiter/pkg/ratelimiter"
)

func main() {
	maxRequestsPerInterval := 100
	requestInterval := time.Second * 5
	var lastSuccess, firstFailure time.Time
	var totalSuccess int
	perClientSlidingWindow := false
	flag.BoolVar(&perClientSlidingWindow, "sliding-window", perClientSlidingWindow, "Enable per client sliding window for rate limiting.")
	flag.Parse()
	fmt.Println("Per client sliding window enabled", perClientSlidingWindow)
	r := ratelimiter.New(time.Duration(requestInterval), maxRequestsPerInterval, perClientSlidingWindow)
	ticker := time.NewTicker(time.Millisecond * 100)
	stopTicker := time.NewTicker(time.Second * 60)
	intervalTickerDuration := time.Duration(requestInterval.Milliseconds()-int64(1000)) * time.Millisecond
	intervalTicker := time.NewTicker(intervalTickerDuration)
	defer stopTicker.Stop()
	defer intervalTicker.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-stopTicker.C:
			fmt.Println("exiting tests...")
			return
		case <-intervalTicker.C:
			if totalSuccess > maxRequestsPerInterval {
				panic(fmt.Sprintf("total requests %d exceeds configured max %d for interval %v", totalSuccess,
					maxRequestsPerInterval, requestInterval))
			}
			fmt.Println("Total success within", maxRequestsPerInterval, "max requests per interval is", totalSuccess)
			totalSuccess = 0
		case <-ticker.C:
			numSuccess := 0
			for i := 0; i < 200; i++ {
				allow := r.ShouldAllow("foobar")
				if !allow {
					numSuccess = 0
					if firstFailure.IsZero() {
						firstFailure = time.Now()
					}
				} else {
					totalSuccess++
					numSuccess++
					if numSuccess > maxRequestsPerInterval {
						panic(fmt.Sprintf("got %d successes. should have rate limited to %d max requests per interval", numSuccess, maxRequestsPerInterval))
					}
					lastSuccess = time.Now()
					if !firstFailure.IsZero() {
						diff := lastSuccess.Sub(firstFailure)
						fmt.Printf("time difference between first failure and last success is %v\n", diff)
						firstFailure = time.Time{}
					}
				}
			}
		}
	}
}
