package utils

import (
	"sync"
	"time"
)

type RateLimiter struct {
	maxRequests int
	timeWindow  time.Duration
	requests    []time.Time
	mutex       sync.Mutex
	logger      *Logger
}

func NewRateLimiter(maxRequests int, timeWindow time.Duration) *RateLimiter {
	return &RateLimiter{
		maxRequests: maxRequests,
		timeWindow:  timeWindow,
		requests:    make([]time.Time, 0),
		logger:      NewLogger("RateLimiter"),
	}
}

func (rl *RateLimiter) Acquire() error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()

	// Remove requests older than the time window
	var validRequests []time.Time
	for _, requestTime := range rl.requests {
		if now.Sub(requestTime) < rl.timeWindow {
			validRequests = append(validRequests, requestTime)
		}
	}
	rl.requests = validRequests

	// Log current status
	rl.logger.Debug("Rate limiter status: %d/%d requests in current window", len(rl.requests), rl.maxRequests)

	// If we're at the limit, wait until we can make another request
	if len(rl.requests) >= rl.maxRequests {
		oldestRequest := rl.requests[0]
		waitTime := rl.timeWindow - now.Sub(oldestRequest) + 200*time.Millisecond // Add 200ms buffer
		
		rl.logger.Info("Rate limit reached. Waiting %v before next request...", waitTime.Round(time.Second))
		
		// Release the lock before sleeping
		rl.mutex.Unlock()
		time.Sleep(waitTime)
		rl.mutex.Lock()
		
		// Recursively try again after waiting
		return rl.acquire()
	}

	// Record this request
	rl.requests = append(rl.requests, now)
	rl.logger.Debug("Request acquired. Current count: %d/%d", len(rl.requests), rl.maxRequests)
	return nil
}

// acquire is the internal method without mutex locking (for recursive calls)
func (rl *RateLimiter) acquire() error {
	now := time.Now()

	// Remove requests older than the time window
	var validRequests []time.Time
	for _, requestTime := range rl.requests {
		if now.Sub(requestTime) < rl.timeWindow {
			validRequests = append(validRequests, requestTime)
		}
	}
	rl.requests = validRequests

	// If we're still at the limit, wait more
	if len(rl.requests) >= rl.maxRequests {
		oldestRequest := rl.requests[0]
		waitTime := rl.timeWindow - now.Sub(oldestRequest) + 200*time.Millisecond
		
		rl.logger.Info("Rate limit still reached. Waiting %v more...", waitTime.Round(time.Second))
		
		rl.mutex.Unlock()
		time.Sleep(waitTime)
		rl.mutex.Lock()
		
		return rl.acquire()
	}

	// Record this request
	rl.requests = append(rl.requests, now)
	rl.logger.Debug("Request acquired after waiting. Current count: %d/%d", len(rl.requests), rl.maxRequests)
	return nil
}

func (rl *RateLimiter) GetStats() (int, int) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	activeRequests := 0
	
	for _, requestTime := range rl.requests {
		if now.Sub(requestTime) < rl.timeWindow {
			activeRequests++
		}
	}
	
	return activeRequests, rl.maxRequests
} 