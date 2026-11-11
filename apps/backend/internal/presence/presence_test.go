package presence

import (
	"testing"
	"time"
)

func TestBucketLimitsNewTabs(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	b := newBucket(2, 3, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !b.allow() {
			t.Fatalf("burst token %d refused", i)
		}
	}
	if b.allow() {
		t.Fatal("allowed beyond the burst")
	}
	now = now.Add(time.Second)
	if !b.allow() || !b.allow() || b.allow() {
		t.Fatal("refill should give exactly 2 tokens per second")
	}
}

func TestCleanLabel(t *testing.T) {
	if got := cleanLabel("  Incidentes \n›\tLentidão\x00 "); got != "Incidentes › Lentidão" {
		t.Fatalf("cleanLabel = %q", got)
	}
}
