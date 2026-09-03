package redpine

import (
	"testing"
	"time"
)

func TestShouldRetry(t *testing.T) {
	for _, s := range []int{200, 400, 401, 403, 404, 422, 500, 502} {
		if shouldRetry(s, 0, 5) {
			t.Fatalf("%d should not retry", s)
		}
	}
	if !shouldRetry(429, 0, 2) || !shouldRetry(429, 1, 2) || shouldRetry(429, 2, 2) || shouldRetry(503, 0, 0) {
		t.Fatal("budget wrong")
	}
}

func TestBackoff(t *testing.T) {
	seven := 7.0
	if backoff(0, &seven, func() float64 { return 0.1 }) != 7*time.Second {
		t.Fatal("Retry-After should win")
	}
	one := func() float64 { return 1 }
	if backoff(0, nil, one) != 500*time.Millisecond || backoff(1, nil, one) != time.Second || backoff(2, nil, one) != 2*time.Second {
		t.Fatal("exponential wrong")
	}
	if backoff(2, nil, func() float64 { return 0 }) != 0 {
		t.Fatal("jitter floor wrong")
	}
	if backoff(20, nil, one) != 30*time.Second {
		t.Fatal("cap wrong")
	}
}
