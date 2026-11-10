// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWeeklyCountsManualFollowUpsOncePerMeeting(t *testing.T) {
	e := setupForecast(t)
	ctx := reportingActor(e)
	at := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	store := activities.NewStore(InstallationDB(e.Pool)).WithClock(func() time.Time { return at })
	meeting, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "meeting", Subject: new("Scope review"), Source: "manual", OccurredAt: &at, MeetingStatus: new("held")})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(meeting.Id)
	for _, subject := range []string{"Send revised scope", "Confirm budget"} {
		if _, _, err := store.LogActivity(ctx, activities.LogActivityInput{Kind: "task", Subject: &subject, Source: "manual", SourceActivityID: &source}); err != nil {
			t.Fatal(err)
		}
	}
	// Creation timestamps come from Postgres, so pin the observation after using the real writer.
	var b reportingBindings
	if _, err := e.owner.Exec(ctx, "UPDATE activity SET created_at="+b.add(at.Add(time.Hour))+" WHERE source_activity_id="+b.add(source), b.values...); err != nil {
		t.Fatal(err)
	}
	if err := InstallationDB(e.Pool).Tx(ctx, func(tx pgx.Tx) error {
		count, err := weeklyConfirmedFollowup(ctx, tx, []reporting.Fact{{SourceID: source}},
			at.Add(24*time.Hour), at.Add(24*time.Hour))
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("meeting next-step count = %d, want one meeting despite two distinct tasks", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
