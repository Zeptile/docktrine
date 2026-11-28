package console

import (
	"testing"
	"time"
)

func TestLeftover(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	started := now.Add(-time.Minute)
	ours := map[string]string{LabelConsoleInstance: "abc"}
	theirs := map[string]string{LabelConsoleInstance: "xyz"}
	owner := Owner{Instance: "abc", Started: started}

	cases := []struct {
		name    string
		owner   Owner
		labels  map[string]string
		created time.Time
		want    bool
	}{
		{"ours, from before this API started", owner, ours, started.Add(-time.Second), true},
		{"ours, from this run", owner, ours, started.Add(time.Second), false},
		{"another installation's, recent", owner, theirs, started.Add(-time.Hour), false},
		{"unlabelled, recent", owner, map[string]string{}, started.Add(-time.Hour), false},
		{"another installation's, older than any session", owner, theirs, now.Add(-DefaultMaxDuration - time.Minute), true},
		{"no owner claims nothing recent", Owner{}, ours, started.Add(-time.Hour), false},
		{"no owner still removes the very old", Owner{}, ours, now.Add(-DefaultMaxDuration - time.Minute), true},
	}
	for _, c := range cases {
		if got := c.owner.Leftover(c.labels, c.created, now); got != c.want {
			t.Errorf("%s: Leftover = %v, want %v", c.name, got, c.want)
		}
	}
}
