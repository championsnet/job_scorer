package multitenant

import (
	"testing"
	"time"
)

func TestComputeNextRunAt(t *testing.T) {
	from := time.Date(2026, 3, 10, 10, 15, 0, 0, time.UTC)
	next, err := computeNextRunAt("0 */1 * * *", "UTC", from)
	if err != nil {
		t.Fatalf("computeNextRunAt returned error: %v", err)
	}
	expected := time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, next)
	}
}

func TestComputeNextRunAt_InvalidCron(t *testing.T) {
	_, err := computeNextRunAt("bad cron", "UTC", time.Now().UTC())
	if err == nil {
		t.Fatalf("expected error for invalid cron expression")
	}
}

func TestComputeNextRunAt_InvalidTimezone(t *testing.T) {
	_, err := computeNextRunAt("0 */1 * * *", "Mars/Phobos", time.Now().UTC())
	if err == nil {
		t.Fatalf("expected error for invalid timezone")
	}
}
