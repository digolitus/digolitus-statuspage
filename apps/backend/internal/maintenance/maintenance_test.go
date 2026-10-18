package maintenance

import "testing"

func TestTransitions(t *testing.T) {
	tests := []struct {
		action Transition
		from   string
		to     string
		ok     bool
	}{
		{Start, StatusScheduled, StatusInProgress, true},
		{Complete, StatusInProgress, StatusCompleted, true},
		{Cancel, StatusScheduled, StatusCancelled, true},
		{Complete, StatusScheduled, "", false},
		{Cancel, StatusInProgress, "", false},
		{Start, StatusInProgress, "", false},
		{Start, StatusCompleted, "", false},
		{Start, StatusCancelled, "", false},
		{Cancel, StatusCompleted, "", false},
	}
	for _, tt := range tests {
		to, ok := Target(tt.action, tt.from)
		if ok != tt.ok || to != tt.to {
			t.Errorf("Target(%s, %s) = %q, %v; want %q, %v", tt.action, tt.from, to, ok, tt.to, tt.ok)
		}
	}
}
