// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// A meeting stays in the week it was held, over a real Postgres.
//
// Weekly counted the activity's current date, so a meeting redated after it was
// held moved to the week somebody edited it into. Analytics counted the held
// transition's own start and did not move it, so one rep could read two figures
// for one week.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldMeetingWeekCount is the held figure a week's assembly would read.
func heldMeetingWeekCount(t *testing.T, e *weekEnv, start time.Time) int {
	t.Helper()
	var held int
	if err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		var err error
		held, _, err = countWeekMeetings(e.repCtx, tx, e.Rep1, afterEveryTransition,
			start, start.AddDate(0, 0, 7))
		return err
	}); err != nil {
		t.Fatalf("counting the week from %v: %v", start, err)
	}
	return held
}

func TestAMeetingRedatedAfterItWasHeldStaysInItsHeldWeek(t *testing.T) {
	e := setupWeekly(t)
	heldWeek := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	laterWeek := heldWeek.AddDate(0, 0, 7)

	held, host := "held", ids.From[ids.UserKind](e.Rep1)
	at := heldWeek.AddDate(0, 0, 2).Add(10 * time.Hour)
	logged, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", MeetingStatus: &held, HostUserID: &host,
		OccurredAt: &at, OccurredPrecision: string(activities.PrecisionInstant),
	})
	if err != nil {
		t.Fatalf("logging the held meeting: %v", err)
	}
	if got := heldMeetingWeekCount(t, e, heldWeek); got != 1 {
		t.Fatalf("the week it was held counts %d, want 1 before any edit", got)
	}

	// Redated through the verb a member uses, which is what makes this the
	// defect's own path rather than a fixture's.
	moved := laterWeek.AddDate(0, 0, 2).Add(10 * time.Hour)
	if _, err := e.Activities.UpdateActivity(e.repCtx, ids.From[ids.ActivityKind](ids.UUID(logged.Id)),
		activities.UpdateActivityInput{OccurredAt: &moved}); err != nil {
		t.Fatalf("redating the meeting: %v", err)
	}

	if got := heldMeetingWeekCount(t, e, heldWeek); got != 1 {
		t.Errorf("the week it was held counts %d after the edit, want 1", got)
	}
	if got := heldMeetingWeekCount(t, e, laterWeek); got != 0 {
		t.Errorf("the week it was moved into counts %d, want 0", got)
	}
}
