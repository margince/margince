// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io"
	"log/slog"
	"math"
	"testing"
	"time"
)

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The two receivers that bound freshness — the extension inbound edge, which
// parses epoch seconds, and the HubSpot receiver, which parses milliseconds —
// share this comparison and nothing else. Held here because a one-directional
// mistake in either copy is invisible from the other: a bound that admitted
// what it should refuse looks, from inside each caller, exactly like a bound
// that works.
func TestWithinSkewIsAbsoluteAndInclusive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"same instant", now, true},
		{"past, inside", now.Add(-time.Minute), true},
		{"future, inside", now.Add(time.Minute), true},
		{"past, exactly at the bound", now.Add(-5 * time.Minute), true},
		{"future, exactly at the bound", now.Add(5 * time.Minute), true},
		{"past, outside", now.Add(-5*time.Minute - time.Second), false},
		{"future, outside", now.Add(5*time.Minute + time.Second), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinSkew(now, tc.at, 5*time.Minute); got != tc.want {
				t.Fatalf("withinSkew = %v, want %v", got, tc.want)
			}
		})
	}
}

// A timestamp far enough ahead to overflow the subtraction is the fast-clock
// sender this bound exists to refuse, and an absolute-value spelling ADMITS it:
// time.Sub saturates at -(1<<63) nanoseconds and that value negates to itself,
// still negative, so `delta <= skew` holds. The window is five minutes and the
// answer for the year 5138 must be the same as for an hour from now.
func TestWithinSkewRefusesATimestampThatOverflowsTheSubtraction(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, secs := range []int64{
		99_999_999_999,  // year 5138
		1 << 62,         // past any calendar
		-99_999_999_999, // and the same distance behind
		math.MinInt64 / 4,
	} {
		at := time.Unix(secs, 0)
		if withinSkew(now, at, 5*time.Minute) {
			t.Errorf("a timestamp at unix %d was judged fresh against a five-minute window", secs)
		}
	}
}
