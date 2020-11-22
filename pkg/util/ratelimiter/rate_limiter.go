package ratelimiter

import (
	"flag"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

func init() {
	flag.DurationVar(&baseDelay, "rate-limiter-base-delay", time.Millisecond*5, "The base delay for rate limiter. Defaults 5ms")
	flag.DurationVar(&maxDelay, "rate-limiter-max-delay", time.Second*1000, "The max delay for rate limiter. Defaults 1000s")
	flag.IntVar(&qps, "rate-limiter-qps", 10, "The qps for rate limier. Defaults 10")
	flag.IntVar(&bucketSize, "rate-limiter-bucket-size", 100, "The bucket size for rate limier. Defaults 100")
}

var baseDelay, maxDelay time.Duration
var qps, bucketSize int

func DefaultControllerRateLimiter() workqueue.RateLimiter {
	return workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(baseDelay, maxDelay),
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(qps), bucketSize)},
	)
}
