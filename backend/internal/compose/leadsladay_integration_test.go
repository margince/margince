// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A day touch answers a lead once its whole date has ended. The subscriber
// skips it while the date runs, so the clock-driven SLA scan is what stamps it.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dayTouchLead is a lead that wrote in at createdAt and a note a rep logged
// for date alone, both through the activity writer.
func dayTouchLead(t *testing.T, e *integration.Env, owner *pgx.Conn, createdAt, date time.Time) ids.UUID {
	t.Helper()
	lead := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, created_at)
		 VALUES ($1, 'Day Lead', 'new', 'inbound', 'human:x', $2, $3)`, lead, e.Rep1, createdAt); err != nil {
		t.Fatal(err)
	}
	store := activities.NewStore(e.DB())
	links := []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead}}
	if _, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: new("Incoming enquiry"), Direction: new("inbound"), OccurredAt: &createdAt, Links: links,
	}); err != nil {
		t.Fatal(err)
	}
	anchor := occurredday.Anchor(date)
	if _, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Called back"), OccurredAt: &anchor,
		OccurredPrecision: activities.PrecisionDay, Source: "manual", Links: links,
	}); err != nil {
		t.Fatal(err)
	}
	return lead
}

// scanSLAAt runs the production scan at now.
func scanSLAAt(t *testing.T, e *integration.Env, now time.Time) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := scanLeadSLA(ctx, e.DB(), func() time.Time { return now }, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("scan at %v: %v", now, err)
	}
}

// leadSLAStamps reads the lead's first response and breach mark.
func leadSLAStamps(t *testing.T, owner *pgx.Conn, lead ids.UUID) (answered, breached *time.Time) {
	t.Helper()
	if err := owner.QueryRow(context.Background(),
		`SELECT first_response_at, sla_breached_at FROM lead WHERE id = $1`, lead).Scan(&answered, &breached); err != nil {
		t.Fatal(err)
	}
	return answered, breached
}

// A date cannot prove an answer within hours, so a note dated the deadline's own
// date leaves the lead breached. Its stored noon, before the deadline, decides nothing.
func TestADayTouchAnswersADeadlineOnlyOnceItsDateHasEnded(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	measureFirstResponse(t, e)
	deadline := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	created := deadline.Add(-15 * time.Minute)
	sameDate := dayTouchLead(t, e, owner, created, deadline)
	dayBefore := dayTouchLead(t, e, owner, created, deadline.AddDate(0, 0, -1))

	scanSLAAt(t, e, deadline.Add(time.Hour))

	if answered, breached := leadSLAStamps(t, owner, sameDate); answered != nil || breached == nil {
		t.Errorf("a note dated the deadline's date: first response %v, breach %v; want none and a breach", answered, breached)
	}
	// The date ended before the lead was created, and no answer predates its lead.
	answered, breached := leadSLAStamps(t, owner, dayBefore)
	if answered == nil || !answered.Equal(created) || breached != nil {
		t.Errorf("a note dated the date before: first response %v, breach %v; want %v and no breach", answered, breached, created)
	}
}

// Once a day touch's date ends, the scan stamps the first response at that end,
// with the target off and on a lead already breached.
func TestTheSLAScanStampsADayTouchOnceItsDateEnds(t *testing.T) {
	date := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	dateEnd := date.AddDate(0, 0, 1)
	for name, measured := range map[string]bool{"the target off": false, "a breached lead": true} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			owner := integration.OwnerConn(t)
			if measured {
				measureFirstResponse(t, e)
			}
			lead := dayTouchLead(t, e, owner, date.Add(9*time.Hour), date)

			scanSLAAt(t, e, date.Add(15*time.Hour))
			answered, breached := leadSLAStamps(t, owner, lead)
			if answered != nil {
				t.Fatalf("the scan stamped %v while the note's date runs", answered)
			}
			if (breached != nil) != measured {
				t.Fatalf("breach %v while the date runs, want a breach only with the target on", breached)
			}

			scanSLAAt(t, e, dateEnd.Add(30*time.Minute))
			answered, after := leadSLAStamps(t, owner, lead)
			if answered == nil || !answered.Equal(dateEnd) {
				t.Errorf("first response %v once the date ended, want its end %v", answered, dateEnd)
			}
			if (after != nil) != measured {
				t.Errorf("breach %v after the late answer, want it kept only where it was marked", after)
			}
		})
	}
}
