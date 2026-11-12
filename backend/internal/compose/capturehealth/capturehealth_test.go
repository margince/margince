// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturehealth

import (
	"testing"
	"time"
)

// A half-recorded gap shows nothing. The two columns carry a CHECK that they
// are set together, so a single end means a row written by something other
// than the sync. A client cannot choose a backfill window from one date.
func TestUnreadPeriodNeedsBothEnds(t *testing.T) {
	at := time.Date(2026, 2, 22, 9, 0, 0, 0, time.UTC)
	later := at.Add(48 * time.Hour)

	for _, c := range []struct {
		name     string
		from, to *time.Time
	}{
		{"neither end", nil, nil},
		{"a start with no end", &at, nil},
		{"an end with no start", nil, &later},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := unreadPeriodOf(c.from, c.to); got != nil {
				t.Errorf("reports the period %v to %v", got.From, got.To)
			}
		})
	}
}

// The wire carries UTC. A mailbox read in a local zone would otherwise shift
// the dates a reader is asked to match a backfill window to.
func TestUnreadPeriodReportsBothEndsInUTC(t *testing.T) {
	zone := time.FixedZone("UTC+7", 7*60*60)
	from := time.Date(2026, 2, 22, 9, 0, 0, 0, zone)
	to := time.Date(2026, 2, 24, 9, 0, 0, 0, zone)

	got := unreadPeriodOf(&from, &to)
	if got == nil {
		t.Fatal("a whole period was not reported")
	}
	if !got.From.Equal(from) || got.From.Location() != time.UTC {
		t.Errorf("the start reads %s, want %s in UTC", got.From, from.UTC())
	}
	if !got.To.Equal(to) || got.To.Location() != time.UTC {
		t.Errorf("the end reads %s, want %s in UTC", got.To, to.UTC())
	}
}
