package expectations

import (
	"flag"
	"time"
)

func init() {
	flag.DurationVar(&ExpectationTimeout, "expectation-timeout", time.Minute*5, "The expectation timeout. Defaults 5min")
}

var ExpectationTimeout time.Duration
