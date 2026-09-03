package redpine

import (
	"math"
	"math/rand"
	"time"
)

var retryableStatuses = map[int]bool{429: true, 503: true}

// shouldRetry: attempt is zero-based (retries already made).
func shouldRetry(status, attempt, maxRetries int) bool {
	return retryableStatuses[status] && attempt < maxRetries
}

const (
	backoffBase   = 500 * time.Millisecond
	backoffFactor = 2.0
	backoffCap    = 30 * time.Second
)

// backoff: Retry-After wins; otherwise exponential with full jitter, capped.
func backoff(attempt int, retryAfter *float64, rnd func() float64) time.Duration {
	if retryAfter != nil {
		return time.Duration(*retryAfter * float64(time.Second))
	}
	if rnd == nil {
		rnd = rand.Float64
	}
	ceiling := math.Min(float64(backoffCap), float64(backoffBase)*math.Pow(backoffFactor, float64(attempt)))
	return time.Duration(ceiling * rnd())
}
