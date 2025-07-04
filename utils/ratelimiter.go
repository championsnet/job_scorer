package utils

import (
	"fmt"
	"sync"
	"time"
)

type RateLimiter struct {
	maxRequests int
	timeWindow  time.Duration
	requests    []time.Time
	mutex       sync.Mutex
	logger      *Logger

	// Token-based limiting
	tokenLimit      int           // Max tokens per window
	tokenWindow     time.Duration // Usually 1 minute
	tokensUsed      int
	tokenWindowStart time.Time
}

func NewRateLimiter(maxRequests int, timeWindow time.Duration) *RateLimiter {
	return &RateLimiter{
		maxRequests: maxRequests,
		timeWindow:  timeWindow,
		requests:    make([]time.Time, 0),
		logger:      NewLogger("RateLimiter"),
	}
}

// NewTokenRateLimiter creates a rate limiter with both request and token limits
func NewTokenRateLimiter(maxRequests int, timeWindow time.Duration, tokenLimit int, tokenWindow time.Duration) *RateLimiter {
	rl := NewRateLimiter(maxRequests, timeWindow)
	rl.tokenLimit = tokenLimit
	rl.tokenWindow = tokenWindow
	rl.tokenWindowStart = time.Now()
	return rl
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
	return nil
}

// AcquireTokens blocks until enough tokens are available for the request
func (rl *RateLimiter) AcquireTokens(tokens int) error {
	maxRetries := 3 // Prevent infinite loops
	
	for retry := 0; retry < maxRetries; retry++ {
		rl.mutex.Lock()
		now := time.Now()
		
		// Reset token window if needed
		if rl.tokenWindowStart.IsZero() || now.Sub(rl.tokenWindowStart) >= rl.tokenWindow {
			rl.tokenWindowStart = now
			rl.tokensUsed = 0
			rl.logger.Info("🔄 Token window reset. Used: 0/%d tokens", rl.tokenLimit)
		}

		// If token limit is not set, just allow
		if rl.tokenLimit == 0 {
			rl.mutex.Unlock()
			return nil
		}

		// Check if enough tokens are available
		if rl.tokensUsed+tokens <= rl.tokenLimit {
			rl.tokensUsed += tokens
			rl.logger.Info("✅ Tokens acquired: %d (total: %d/%d)", tokens, rl.tokensUsed, rl.tokenLimit)
			rl.mutex.Unlock()
			return nil
		}

		// Calculate wait time until next window
		timeIntoWindow := now.Sub(rl.tokenWindowStart)
		waitTime := rl.tokenWindow - timeIntoWindow + 200*time.Millisecond
		
		if waitTime <= 0 {
			// Window should have reset, try again immediately
			rl.logger.Warning("⚠️ Negative wait time calculated, resetting window")
			rl.tokenWindowStart = now
			rl.tokensUsed = 0
			rl.mutex.Unlock()
			continue
		}

		rl.logger.Info("🛑 Token limit reached (%d/%d). Waiting %v for window reset...", 
			rl.tokensUsed, rl.tokenLimit, waitTime.Round(time.Second))
		
		rl.mutex.Unlock()
		time.Sleep(waitTime)
		
		// After waiting, the next iteration will reset the window and try again
	}
	
	return fmt.Errorf("token rate limiter failed after %d retries", maxRetries)
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
	return nil
}

func (rl *RateLimiter) GetStats() (int, int, int, int) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	activeRequests := 0
	for _, requestTime := range rl.requests {
		if now.Sub(requestTime) < rl.timeWindow {
			activeRequests++
		}
	}

	tokensUsed := rl.tokensUsed
	tokenLimit := rl.tokenLimit
	if rl.tokenWindowStart.IsZero() || now.Sub(rl.tokenWindowStart) >= rl.tokenWindow {
		tokensUsed = 0
	}

	return activeRequests, rl.maxRequests, tokensUsed, tokenLimit
}

// HandleRetryAfter implements additional waiting based on server retry-after headers
func (rl *RateLimiter) HandleRetryAfter(seconds int) {
	if seconds <= 0 {
		return
	}
	
	waitTime := time.Duration(seconds) * time.Second
	rl.logger.Info("🚫 Server requested retry-after: %d seconds. Waiting %v...", seconds, waitTime)
	time.Sleep(waitTime)
}

// UpdateFromHeaders updates rate limiter state based on server response headers
func (rl *RateLimiter) UpdateFromHeaders(remainingTokens, resetTokensSeconds int) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	if remainingTokens >= 0 && rl.tokenLimit > 0 {
		rl.tokensUsed = rl.tokenLimit - remainingTokens
		rl.logger.Info("📊 Updated from headers: %d/%d tokens used", rl.tokensUsed, rl.tokenLimit)
		
		if remainingTokens < 500 { // Very low on tokens
			rl.logger.Warning("⚠️ Very low tokens remaining: %d", remainingTokens)
		}
	}
} 