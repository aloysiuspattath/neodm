package engine

import (
	"context"
	"sync"
	"time"
)

// RateLimiter provides token-bucket bandwidth throttling.
type RateLimiter struct {
	mu           sync.Mutex
	bytesPerSec  int64
	tokens       int64
	lastRefill   time.Time
	bucketCap    int64
}

// NewRateLimiter creates a limiter with the specified bytes per second limit (0 = unlimited).
func NewRateLimiter(bytesPerSec int64) *RateLimiter {
	return &RateLimiter{
		bytesPerSec: bytesPerSec,
		tokens:      bytesPerSec,
		lastRefill:  time.Now(),
		bucketCap:   bytesPerSec * 2, // 2-second burst tolerance
	}
}

// SetLimit dynamically updates the throttle rate (0 = unlimited).
func (r *RateLimiter) SetLimit(bytesPerSec int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bytesPerSec = bytesPerSec
	r.bucketCap = bytesPerSec * 2
	r.tokens = bytesPerSec
	r.lastRefill = time.Now()
}

// GetLimit returns the current bandwidth limit in bytes/sec.
func (r *RateLimiter) GetLimit() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytesPerSec
}

// Wait blocks until the requested number of bytes can be transferred or context cancelled.
func (r *RateLimiter) Wait(ctx context.Context, bytes int64) error {
	r.mu.Lock()
	if r.bytesPerSec <= 0 {
		r.mu.Unlock()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			r.mu.Unlock()
			return ctx.Err()
		default:
		}

		now := time.Now()
		elapsed := now.Sub(r.lastRefill).Seconds()
		r.lastRefill = now

		// Refill tokens
		r.tokens += int64(elapsed * float64(r.bytesPerSec))
		if r.tokens > r.bucketCap {
			r.tokens = r.bucketCap
		}

		if r.tokens >= bytes {
			r.tokens -= bytes
			r.mu.Unlock()
			return nil
		}

		// Calculate wait duration
		needed := bytes - r.tokens
		waitSec := float64(needed) / float64(r.bytesPerSec)
		if waitSec > 0.5 {
			waitSec = 0.5 // Cap sleep interval for responsiveness
		}
		sleepDuration := time.Duration(waitSec * float64(time.Second))
		if sleepDuration < 5*time.Millisecond {
			sleepDuration = 5 * time.Millisecond
		}

		r.mu.Unlock()
		time.Sleep(sleepDuration)
		r.mu.Lock()
	}
}
