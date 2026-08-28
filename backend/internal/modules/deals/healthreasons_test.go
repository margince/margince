// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"
	"time"
)

// TestTheRecencySentenceCountsTheWayTheCacheKeyDoes is the defect this closes,
// stated as the reader meets it.
//
// The deal status card is cached on a fingerprint that includes the UTC day. A
// sentence counting ELAPSED hours changes while that key stands still, so the
// card spends part of a day saying something the records do not support and
// nothing invalidates it. Counting by the calendar means the words can only
// change when the key does.
func TestTheRecencySentenceCountsTheWayTheCacheKeyDoes(t *testing.T) {
	last := time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC)
	for _, probe := range []struct {
		what string
		now  time.Time
		want string
	}{
		// The reported case: two calendar days later, before and after the hour
		// the elapsed count would tick on. Both are "2 days ago", and that is
		// the whole point — the sentence must not change between them.
		{"the morning of the 22nd", time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC), "2 days ago"},
		{"the same afternoon", time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC), "2 days ago"},
		{"that evening", time.Date(2026, 8, 22, 23, 59, 0, 0, time.UTC), "2 days ago"},
		// A reader looking at two dates counts the calendar, not the clock: an
		// hour before midnight to an hour after is yesterday, not today.
		{"an hour after midnight the next day", time.Date(2026, 8, 21, 1, 0, 0, 0, time.UTC), "yesterday"},
		{"later the same day", time.Date(2026, 8, 20, 23, 0, 0, 0, time.UTC), "today"},
	} {
		t.Run(probe.what, func(t *testing.T) {
			got := recencyReason(&last, false, probe.now)
			if !strings.Contains(got, probe.want) {
				t.Errorf("at %s the card says %q, want it to say %q — a sentence a reader can check "+
					"against a timestamp must not move between cache writes",
					probe.now.Format(time.RFC3339), got, probe.want)
			}
		})
	}
}

// A clock behind the record — a fixed test clock, a row seeded ahead — reads as
// today rather than as a negative count. The card has nothing else it could
// honestly say about an activity that has not happened yet.
func TestARecordAheadOfTheClockReadsAsToday(t *testing.T) {
	last := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	got := recencyReason(&last, false, time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC))
	if !strings.Contains(got, "today") {
		t.Errorf("an activity dated ahead of now reads %q, want today", got)
	}
}

// The stalled clause rides the same sentence, so a change to how the days are
// counted must not lose it.
func TestAStalledDealSaysSoInTheSameSentence(t *testing.T) {
	last := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	got := recencyReason(&last, true, time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC))
	if !strings.Contains(got, "21 days ago") || !strings.Contains(got, "stalled") {
		t.Errorf("a stalled deal's recency reads %q, want the day count and the stalled clause", got)
	}
}
