// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"
	"time"
)

// The report and the summary refuse a usage window by one rule, because both
// aggregate every metered call in it.
func TestAUsageWindowIsInOrderAndAtMostAYear(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		to   time.Time
		code string
	}{
		{"a month", from.AddDate(0, 1, 0), ""},
		{"exactly the cap", from.Add(maxUsageWindowDays * 24 * time.Hour), ""},
		{"a day past the cap", from.Add((maxUsageWindowDays + 1) * 24 * time.Hour), "window_too_wide"},
		{"before it starts", from.AddDate(0, 0, -1), "window_inverted"},
	} {
		refusal := usageWindowRefusal(from, c.to)
		got := ""
		if refusal != nil {
			got = refusal.Code
		}
		if got != c.code {
			t.Errorf("%s: refusal %q, want %q", c.name, got, c.code)
		}
	}
}
