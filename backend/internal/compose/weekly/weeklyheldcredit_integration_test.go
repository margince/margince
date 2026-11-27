// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A colleague who attended a meeting keeps it in the week it was held. Losing
// their guest row later changes nothing, because the held transition kept them.
func TestAColleagueKeepsAHeldMeetingInItsWeekWhenTheirGuestRowIsLost(t *testing.T) {
	e := setupWeekly(t)
	week := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	held, host := "held", ids.From[ids.UserKind](e.Rep1)
	at := week.AddDate(0, 0, 2).Add(10 * time.Hour)
	logged, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", MeetingStatus: &held, HostUserID: &host,
		AttendeeUserIDs: []ids.UserID{ids.From[ids.UserKind](e.Rep2)},
		OccurredAt:      &at, OccurredPrecision: string(activities.PrecisionInstant),
	})
	if err != nil {
		t.Fatalf("logging the held meeting: %v", err)
	}
	count := func() int {
		var got int
		if err := database.WithTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
			var err error
			got, _, err = countWeekMeetings(e.repCtx, tx, e.Rep2, week, week.AddDate(0, 0, 7))
			return err
		}); err != nil {
			t.Fatalf("counting the week: %v", err)
		}
		return got
	}
	if got := count(); got != 1 {
		t.Fatalf("the guest's held week counts %d, want 1 with the guest row present", got)
	}
	e.WsExec(t, `DELETE FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, ids.UUID(logged.Id), e.Rep2)
	if got := count(); got != 1 {
		t.Errorf("after the guest row was lost the guest's held week counts %d, want 1", got)
	}
}
