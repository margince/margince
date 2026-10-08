// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"testing"
	"time"
)

// The worker's client never schedules River's reindexer.
func TestTheWorkersClientDoesNotScheduleTheReindexer(t *testing.T) {
	schedule := riverConfig(Config{}, nil).ReindexerSchedule
	if schedule == nil {
		t.Fatal("no reindexer schedule is set, so River rebuilds its indexes at midnight UTC under a role that does not own them")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if next := schedule.Next(now); next.Before(now.AddDate(100, 0, 0)) {
		t.Errorf("the reindexer is next scheduled for %s; it must never run on this client", next)
	}
}
