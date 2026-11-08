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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The week leaves out a cancellation a repair reversed. The meeting counts as
// booked once, not again for its reopening.
func TestAReversedCalendarCancellationCountsNoSecondBooking(t *testing.T) {
	e := setupWeekly(t)
	seedLeadFor(t, e, "Funnel", e.Rep1)
	booked, host := "booked", ids.From[ids.UserKind](e.Rep1)
	at := time.Now().UTC().Add(48 * time.Hour)
	meeting, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", MeetingStatus: &booked, HostUserID: &host, OccurredAt: &at,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	key := connector.NaturalKey{SourceSystem: "gcal", SourceID: "evt-guest-copy"}
	cancelling := e.ConnectorOwnerCtx(e.Rep1, "gcal")
	if err := database.WithWorkspaceTx(cancelling, e.Pool, func(tx pgx.Tx) error {
		_, err := activities.CancelMeetingByIDTx(cancelling, tx, id, key, at)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	repair := principal.SystemActing(e.Admin(), "system:datatask:test")
	if err := database.WithWorkspaceTx(repair, e.Pool, func(tx pgx.Tx) error {
		_, err := activities.ReopenCalendarCancellationTx(repair, tx, activities.CalendarCancellation{
			ID: id.UUID, ActorType: "connector", Restore: &booked, Key: key,
		}, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	if review.Scorecard == nil || review.Scorecard.Lead == nil {
		t.Fatal("a rep with a lead has a funnel block")
	}
	if got := review.Scorecard.Lead.Booked; got != 1 {
		t.Fatalf("the week counts %d bookings, want the one the meeting was booked with", got)
	}
}
