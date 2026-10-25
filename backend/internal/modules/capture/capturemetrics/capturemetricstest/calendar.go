// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturemetricstest reads the calendar counters back out of the
// exposition, for the tests of the connectors that increment them.
package capturemetricstest

import (
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// CalendarCounts answers how many events provider has had listed and how many
// ended on each outcome. The counters are process-wide, so a test reads them
// before and after, under a provider name no other test in its package uses.
func CalendarCounts(t testing.TB, provider string) (listed int, outcomes map[string]int) {
	t.Helper()
	var b strings.Builder
	capturemetrics.WriteProcessMetrics(&b)
	listedLine := "margince_capture_calendar_events_listed_total{provider=" + httpserver.Label(provider) + "} "
	outcomePrefix := "margince_capture_calendar_events_total{provider=" + httpserver.Label(provider) + ",outcome="
	outcomes = map[string]int{}
	for line := range strings.SplitSeq(b.String(), "\n") {
		if value, ok := strings.CutPrefix(line, listedLine); ok {
			listed = count(t, line, value)
			continue
		}
		rest, ok := strings.CutPrefix(line, outcomePrefix)
		if !ok {
			continue
		}
		label, value, _ := strings.Cut(rest, "} ")
		outcomes[strings.Trim(label, `"`)] = count(t, line, value)
	}
	return listed, outcomes
}

// Moved answers how far each count moved between two reads.
func Moved(before, after map[string]int) map[string]int {
	moved := map[string]int{}
	for outcome, n := range after {
		if delta := n - before[outcome]; delta != 0 {
			moved[outcome] = delta
		}
	}
	return moved
}

// Total is the number of events the outcomes in moved add up to.
func Total(moved map[string]int) int {
	total := 0
	for _, n := range moved {
		total += n
	}
	return total
}

func count(t testing.TB, line, value string) int {
	t.Helper()
	n, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("reading %q: %v", line, err)
	}
	return n
}
