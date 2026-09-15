package exceptions

import (
	"testing"
	"time"
)

func TestWorkflow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	for _, tc := range []struct {
		from, to State
		reason   string
		expiry   *time.Time
		valid    bool
	}{{Open, InReview, "investigate", nil, true}, {Reopened, InReview, "again", nil, true}, {InReview, Accepted, "confirmed", nil, true}, {InReview, Dismissed, "not an issue", nil, true}, {InReview, Suppressed, "approved exception", &future, true}, {InReview, Suppressed, "expired", &past, false}, {InReview, Suppressed, "no expiry", nil, false}, {Open, Resolved, "manual close", nil, false}, {Dismissed, Open, "reset", nil, false}, {Open, InReview, "", nil, false}, {Open, InReview, "invalid expiry", &future, false}} {
		if got := Transition(tc.from, tc.to, tc.reason, tc.expiry, now) == nil; got != tc.valid {
			t.Errorf("%s -> %s valid=%v", tc.from, tc.to, got)
		}
	}
}

func TestThreeRunsAndSuppression(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	for _, tc := range []struct {
		prior    State
		present  bool
		old, new string
		expiry   *time.Time
		want     State
		class    string
	}{{"", true, "", "h", nil, Open, "new"}, {Dismissed, true, "h", "h", nil, Dismissed, "still_open"}, {Accepted, true, "h", "new", nil, Accepted, "still_open"}, {Open, false, "h", "", nil, Resolved, "resolved_in_source"}, {Dismissed, false, "h", "", nil, Dismissed, "absent"}, {Suppressed, true, "h", "h", &future, Suppressed, "suppressed"}, {Suppressed, true, "h", "h", &past, Reopened, "reopened"}, {Suppressed, true, "h", "h", nil, Reopened, "reopened"}, {Suppressed, true, "h", "new", &future, Reopened, "reopened_changed"}, {Resolved, true, "h", "h", nil, Reopened, "reopened"}} {
		s, c := Reconcile(tc.prior, tc.present, tc.old, tc.new, tc.expiry, now)
		if s != tc.want || c != tc.class {
			t.Errorf("reconcile %s: got %s %s", tc.prior, s, c)
		}
	}
}
