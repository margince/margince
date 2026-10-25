// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// seedDayNote links a note a rep logged for date alone, stored at its anchor.
func (e *promoteConsentEnv) seedDayNote(t *testing.T, lead ids.LeadID, date time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, occurred_precision, source, captured_by)
		 VALUES ($1, 'note', 'Called back', $2, 'day', 'manual', $3)`,
		id, occurredday.Anchor(date), "human:"+e.user.String()); err != nil {
		t.Fatalf("seeding the day note: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, id, lead); err != nil {
		t.Fatalf("linking the day note to the lead: %v", err)
	}
	return id
}

// A day touch answers a response deadline only once its whole date has ended
// by it: a date cannot prove an answer within hours. Its stored noon, before
// the deadline here, decides nothing.
func TestADayTouchAnswersADeadlineOnlyOnceItsDateHasEnded(t *testing.T) {
	e := setupPromoteConsent(t)
	e.enableFirstResponseSLA(t)
	deadline := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	for name, c := range map[string]struct {
		date     time.Time
		answered bool
	}{
		"a note dated the deadline's own date": {deadline, false},
		"a note dated the date before":         {deadline.AddDate(0, 0, -1), true},
	} {
		t.Run(name, func(t *testing.T) {
			lead := e.seedLeadCreatedAt(t, c.date.Format(time.DateOnly)+"@sla.test", deadline.Add(-DefaultFirstResponseTarget))
			e.seedDayNote(t, lead, c.date)

			breaches, err := e.store.ScanLeadSLA(e.ctx, deadline.Add(18*time.Hour))
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			breached := false
			for _, b := range breaches {
				breached = breached || b.LeadID == lead
			}
			if breached == c.answered {
				t.Fatalf("breached = %v, want %v", breached, !c.answered)
			}
			after, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			stamped := after.FirstResponseAt != nil && !after.FirstResponseAt.After(deadline)
			if stamped != c.answered || (!c.answered && after.FirstResponseAt != nil) {
				t.Errorf("first_response_at = %v beside a %v breach; the stamp and the scan disagree", after.FirstResponseAt, breached)
			}
		})
	}
}

// The subscriber stamps a day touch only once its date has ended, at that end.
// Stamped earlier, the lead would stop owing a reply before the date that could
// prove the answer is over.
func TestTheFirstResponseStampWaitsForADayTouchsDateToEnd(t *testing.T) {
	e := setupPromoteConsent(t)
	now := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	previousClock := leadSLAClock
	leadSLAClock = func() time.Time { return now }
	t.Cleanup(func() { leadSLAClock = previousClock })
	lead := e.seedLeadCreatedAt(t, "today@sla.test", now.Add(-time.Hour))
	note := e.seedDayNote(t, lead, now)
	subscriber := leadFirstResponse{store: e.store}
	ev := workflow.Event{
		ID: ids.NewV7(), Type: "activity.captured", WorkspaceID: e.ws, OccurredAt: now,
		Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: note},
	}
	stampAt := func() *time.Time {
		t.Helper()
		eff, err := subscriber.Plan(e.ctx, ev)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := subscriber.Apply(e.ctx, ev, eff, nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
		read, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		return read.FirstResponseAt
	}

	if got := stampAt(); got != nil {
		t.Fatalf("a day note dated today stamped the first response at %v while its date runs", got)
	}
	leadSLAClock = func() time.Time { return now.Add(24 * time.Hour) }
	if got, want := stampAt(), time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC); got == nil || !got.Equal(want) {
		t.Errorf("once its date ended the stamp is %v, want the date's end %v", got, want)
	}
}
