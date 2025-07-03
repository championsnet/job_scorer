package utils

import (
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name        string
		maxRequests int
		timeWindow  time.Duration
	}{
		{
			name:        "Valid rate limiter",
			maxRequests: 10,
			timeWindow:  time.Minute,
		},
		{
			name:        "Small rate limiter",
			maxRequests: 1,
			timeWindow:  time.Second,
		},
		{
			name:        "High rate limiter",
			maxRequests: 100,
			timeWindow:  time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.maxRequests, tt.timeWindow)
			
			if rl == nil {
				t.Errorf("NewRateLimiter() returned nil rate limiter")
			}
			
			// Verify configuration through GetStats
			_, max := rl.GetStats()
			if max != tt.maxRequests {
				t.Errorf("NewRateLimiter() maxRequests = %d, want %d", max, tt.maxRequests)
			}
		})
	}
}

func TestRateLimiterAcquire(t *testing.T) {
	// Create a rate limiter that allows 2 requests per 200ms
	rl := NewRateLimiter(2, 200*time.Millisecond)

	start := time.Now()

	// First two requests should be immediate
	err := rl.Acquire()
	if err != nil {
		t.Errorf("First Acquire() error = %v", err)
	}

	err = rl.Acquire()
	if err != nil {
		t.Errorf("Second Acquire() error = %v", err)
	}

	// Check that the first two requests were fast
	if time.Since(start) > 50*time.Millisecond {
		t.Errorf("First two requests took too long: %v", time.Since(start))
	}

	// Check stats after two requests
	active, max := rl.GetStats()
	if active != 2 {
		t.Errorf("GetStats() active = %d, want 2", active)
	}
	if max != 2 {
		t.Errorf("GetStats() max = %d, want 2", max)
	}
}

func TestRateLimiterGetStats(t *testing.T) {
	// Create a rate limiter that allows 3 requests per 100ms
	rl := NewRateLimiter(3, 100*time.Millisecond)

	// Initially should have no active requests
	active, max := rl.GetStats()
	if active != 0 {
		t.Errorf("GetStats() initial active = %d, want 0", active)
	}
	if max != 3 {
		t.Errorf("GetStats() max = %d, want 3", max)
	}

	// Make one request
	err := rl.Acquire()
	if err != nil {
		t.Errorf("Acquire() error = %v", err)
	}

	// Should have 1 active request
	active, max = rl.GetStats()
	if active != 1 {
		t.Errorf("GetStats() after one request active = %d, want 1", active)
	}
	if max != 3 {
		t.Errorf("GetStats() max = %d, want 3", max)
	}

	// Make two more requests
	rl.Acquire()
	rl.Acquire()

	// Should have 3 active requests
	active, max = rl.GetStats()
	if active != 3 {
		t.Errorf("GetStats() after three requests active = %d, want 3", active)
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	// Create a rate limiter that allows 2 requests per 100ms
	rl := NewRateLimiter(2, 100*time.Millisecond)

	const numGoroutines = 4
	done := make(chan error, numGoroutines)

	// Start multiple goroutines making requests
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			err := rl.Acquire()
			done <- err
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-done:
			if err == nil {
				successCount++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Test timed out - possible deadlock")
		}
	}

	// At least some requests should succeed
	if successCount == 0 {
		t.Error("No requests succeeded in concurrent test")
	}

	t.Logf("Concurrent test: %d/%d requests succeeded", successCount, numGoroutines)
}

func TestRateLimiterTimeWindow(t *testing.T) {
	// Create a rate limiter that allows 1 request per 100ms
	rl := NewRateLimiter(1, 100*time.Millisecond)

	// Make first request (should be immediate)
	start := time.Now()
	err := rl.Acquire()
	if err != nil {
		t.Errorf("First request error = %v", err)
	}

	// Should be fast for the first request
	if time.Since(start) > 10*time.Millisecond {
		t.Errorf("First request took too long: %v", time.Since(start))
	}

	// Check that we have 1 active request
	active, max := rl.GetStats()
	if active != 1 {
		t.Errorf("After first request, active = %d, want 1", active)
	}
	if max != 1 {
		t.Errorf("Max requests = %d, want 1", max)
	}

	// Wait for time window to pass
	time.Sleep(150 * time.Millisecond)

	// Now the first request should have expired
	active, _ = rl.GetStats()
	if active != 0 {
		t.Errorf("After time window, active = %d, want 0", active)
	}
} 