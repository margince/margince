// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The day-answer sweep reads only the leads a day touch can still answer, so a
// lead it can never stamp is not read again on every scan.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// seedDayTouch links a day entry dated date to lead, as a capture commits it.
func (e *promoteConsentEnv) seedDayTouch(t *testing.T, lead ids.LeadID, kind, direction string, date time.Time) {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity (id, kind, subject, direction, occurred_at, occurred_precision, source, captured_by)
		 VALUES ($1, $2, 'Touch', $3, $4, 'day', 'manual', $5)`,
		id, kind, direction, occurredday.Anchor(date), "human:"+e.user.String()); err != nil {
		t.Fatalf("seeding the day %s: %v", kind, err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, id, lead); err != nil {
		t.Fatalf("linking the day %s to the lead: %v", kind, err)
	}
}

// seedBareLead is a lead typed in by hand: no enquiry is linked to it.
func (e *promoteConsentEnv) seedBareLead(t *testing.T, email string, createdAt time.Time) ids.LeadID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO lead (id, full_name, email, status, source, captured_by, owner_id, created_at)
		 VALUES ($1, 'Hand Lead', lower($2), 'new', 'manual', 'human:x', $3, $4)`,
		id, email, e.user, createdAt); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.LeadKind](id)
}

func (e *promoteConsentEnv) dayAnswerCandidatesAt(t *testing.T, now time.Time, after ids.LeadID) []ids.LeadID {
	t.Helper()
	var leads []ids.LeadID
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		leads, err = dayAnswerCandidates(e.ctx, tx, now, after)
		return err
	}); err != nil {
		t.Fatalf("read the day-answer candidates: %v", err)
	}
	return leads
}

// A lead whose only ended day touch is its own enquiry has nothing a scan can
// stamp, so the sweep never reads it. A lead with no enquiry and a day call is
// read, as the subscriber would stamp it.
func TestTheDaySweepReadsOnlyLeadsADayTouchCanAnswer(t *testing.T) {
	e := setupPromoteConsent(t)
	date := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	enquiryOnly := e.seedBareLead(t, "enquiry@day.test", date)
	e.seedDayTouch(t, enquiryOnly, "email", "inbound", date)
	called := e.seedBareLead(t, "called@day.test", date)
	e.seedDayTouch(t, called, "call", "outbound", date)

	leads := e.dayAnswerCandidatesAt(t, date.AddDate(0, 0, 1).Add(time.Hour), ids.LeadID{})
	if slices.Contains(leads, enquiryOnly) {
		t.Error("the sweep reads a lead whose only ended day touch is its own enquiry, on every scan")
	}
	if !slices.Contains(leads, called) {
		t.Error("the sweep skips a lead with no enquiry and an ended day call, which the subscriber would stamp")
	}
	if got := e.dayAnswerCandidatesAt(t, date.Add(15*time.Hour), ids.LeadID{}); slices.Contains(got, called) {
		t.Error("the sweep reads a day call while its date still runs")
	}
	if got := e.dayAnswerCandidatesAt(t, date.AddDate(0, 0, 2), called); slices.Contains(got, called) {
		t.Error("a page starting after the lead reads it again")
	}

	breaches, err := e.store.ScanLeadSLA(e.ctx, date.AddDate(0, 0, 1).Add(time.Hour))
	if err != nil || len(breaches) != 0 {
		t.Fatalf("scan: %v, breaches %v", err, breaches)
	}
	if got := e.dayAnswerCandidatesAt(t, date.AddDate(0, 0, 2), ids.LeadID{}); slices.Contains(got, called) {
		t.Error("the sweep reads the called lead again after stamping it")
	}
}

// mayAnswerSQL is a necessary condition of isFirstResponseActivity: every touch
// the Go rule accepts, the SQL admits. Postgres judges each touch of the grid.
func TestMayAnswerSQLAdmitsEveryAcceptedTouch(t *testing.T) {
	e := setupPromoteConsent(t)
	accepted, refused := 0, 0
	for _, direction := range []string{"", "inbound", touchOutbound} {
		for _, kind := range []string{"email", "call", "meeting", touchNote} {
			for _, source := range []string{activitySourceManual, "connector"} {
				for _, capturedBy := range []string{humanCapturedPrefix + "x", "agent:x", "system:x"} {
					touch := leadResponseTouch{direction: direction, kind: kind, source: source, capturedBy: capturedBy, hadInbound: true}
					var admits bool
					if err := e.owner.QueryRow(context.Background(), `SELECT coalesce(`+mayAnswerSQL("a")+`, false)
						FROM (VALUES (NULLIF($1, ''), $2::text, $3::text, $4::text)) AS a(direction, kind, source, captured_by)`,
						direction, kind, source, capturedBy).Scan(&admits); err != nil {
						t.Fatal(err)
					}
					if isFirstResponseActivity(touch) {
						accepted++
						if !admits {
							t.Errorf("mayAnswerSQL refuses %+v, which isFirstResponseActivity accepts", touch)
						}
					} else if !admits {
						refused++
					}
				}
			}
		}
	}
	if accepted == 0 || refused == 0 {
		t.Fatalf("the grid holds %d accepted and %d refused touches; it must hold both", accepted, refused)
	}
}
