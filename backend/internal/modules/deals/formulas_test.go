// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// §8 fixed-clock table: the stalled boolean over status, idle duration
// and the wait suppression.

import (
	"testing"
	"time"
)

func TestIsStalled(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	at := func(daysAgo int) *time.Time {
		v := now.AddDate(0, 0, -daysAgo)
		return &v
	}

	cases := []struct {
		name    string
		status  string
		created time.Time
		lastAct *time.Time
		wait    *time.Time
		want    bool
	}{
		{"fresh open deal", "open", now.AddDate(0, 0, -5), at(2), nil, false},
		{"idle past threshold", "open", now.AddDate(0, 0, -90), at(61), nil, true},
		{"exactly at threshold is not stalled", "open", now.AddDate(0, 0, -90), at(60), nil, false},
		{"no activity ever, old deal", "open", now.AddDate(0, 0, -61), nil, nil, true},
		{"closed deals never stall", "won", now.AddDate(0, 0, -400), at(300), nil, false},
		{"active wait suppresses", "open", now.AddDate(0, 0, -90), at(80), at(-10), false},
		{"expired wait un-suppresses", "open", now.AddDate(0, 0, -90), at(80), at(5), true},
	}
	for _, c := range cases {
		if got := IsStalled(c.status, c.created, c.lastAct, c.wait, now); got != c.want {
			t.Errorf("%s: IsStalled = %v, want %v", c.name, got, c.want)
		}
	}
}

// The shorter window answers the same question with less patience. A deal quiet
// three weeks is not stalled, and the two claims must not be able to disagree
// about anything except the number — which is why the stalled spelling is the
// quiet one at its own threshold rather than a second rule.
func TestIsQuietForIsTheSameRuleAtADifferentWindow(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	at := func(daysAgo int) *time.Time {
		v := now.AddDate(0, 0, -daysAgo)
		return &v
	}
	created := now.AddDate(0, 0, -90)

	// Twenty days idle: quiet at the shorter window, not yet stalled.
	if !IsQuietFor(QuietThresholdDays, "open", created, at(20), nil, now) {
		t.Error("a deal idle 20 days is not quiet at the 19-day window, want quiet")
	}
	if IsStalled("open", created, at(20), nil, now) {
		t.Error("a deal idle 20 days reads as stalled, want not stalled")
	}

	// Every suppression the stalled rule keeps, the shorter window keeps too.
	if IsQuietFor(QuietThresholdDays, "won", created, at(300), nil, now) {
		t.Error("a closed deal reads as quiet, want never")
	}
	if IsQuietFor(QuietThresholdDays, "open", created, at(80), at(-10), now) {
		t.Error("an active wait failed to suppress quiet, want suppressed")
	}

	// The stalled spelling IS the quiet one at its own threshold. Asserted over
	// the same table the rule is specified by, so a change to either that made
	// them disagree fails here rather than in whichever surface noticed first.
	for _, idle := range []int{0, 5, 18, 19, 20, 59, 60, 61, 200} {
		if got, want := IsQuietFor(StalledThresholdDays, "open", created, at(idle), nil, now),
			IsStalled("open", created, at(idle), nil, now); got != want {
			t.Errorf("idle %dd: IsQuietFor(stalled window) = %v, IsStalled = %v — one rule, two answers",
				idle, got, want)
		}
	}
}
