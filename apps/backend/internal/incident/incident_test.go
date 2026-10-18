package incident

import "testing"

func TestCanTransition(t *testing.T) {
	for _, from := range OpenStatuses {
		for _, to := range Statuses {
			if !CanTransition(from, to) {
				t.Errorf("%s → %s should be allowed", from, to)
			}
		}
	}
	for _, to := range Statuses {
		if CanTransition(StatusResolved, to) {
			t.Errorf("resolved is terminal; %s allowed", to)
		}
	}
	if CanTransition("bogus", StatusResolved) {
		t.Error("unknown source status must be rejected")
	}
}
