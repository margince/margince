// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The day-answer sweep reads only the leads a day touch can still answer. A
// lead it can never stamp is not read again on every scan: its own enquiry,
// or an agent's outbound before the lead wrote in.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// seedDayTouch links a day entry a rep logged, dated date, to lead. An empty
// direction is a note's, which has none.
func (e *promoteConsentEnv) seedDayTouch(t *testing.T, lead ids.LeadID, kind, direction string, date time.Time) {
	t.Helper()
	e.seedDayTouchBy(t, lead, kind, direction, "human:"+e.user.String(), date)
}

// seedDayTouchBy is seedDayTouch for the author capturedBy.
func (e *promoteConsentEnv) seedDayTouchBy(t *testing.T, lead ids.LeadID, kind, direction, capturedBy string, date time.Time) {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity (id, kind, subject, direction, occurred_at, occurred_precision, source, captured_by)
		 VALUES ($1, $2, 'Touch', NULLIF($3, ''), $4, 'day', 'manual', $5)`,
		id, kind, direction, occurredday.Anchor(date), capturedBy); err != nil {
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
	agentWrote := e.seedBareLead(t, "agent@day.test", date)
	e.seedDayTouchBy(t, agentWrote, "email", "outbound", "agent:x", date)

	leads := e.dayAnswerCandidatesAt(t, date.AddDate(0, 0, 1).Add(time.Hour), ids.LeadID{})
	if slices.Contains(leads, enquiryOnly) {
		t.Error("the sweep reads a lead whose only ended day touch is its own enquiry, on every scan")
	}
	if slices.Contains(leads, agentWrote) {
		t.Error("the sweep reads a lead whose only ended day touch is an agent's outbound before it wrote in, on every scan")
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

// mayAnswerSQL is isFirstResponseActivity in SQL. Postgres judges each touch of
// the grid as the Go rule does, for a lead that wrote in and one that did not.
func TestMayAnswerSQLAgreesWithTheGoRule(t *testing.T) {
	e := setupPromoteConsent(t)
	date := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	wroteIn := e.seedBareLead(t, "wrote@day.test", date)
	e.seedDayTouch(t, wroteIn, "email", "inbound", date)
	cold := e.seedBareLead(t, "cold@day.test", date)
	accepted, refused := 0, 0
	for _, direction := range []string{"", "inbound", touchOutbound} {
		for _, kind := range []string{"email", "call", "meeting", touchNote} {
			for _, source := range []string{activitySourceManual, "connector"} {
				for _, capturedBy := range []string{humanCapturedPrefix + "x", "agent:x", "system:x"} {
					for lead, hadInbound := range map[ids.LeadID]bool{wroteIn: true, cold: false} {
						touch := leadResponseTouch{direction: direction, kind: kind, source: source, capturedBy: capturedBy, hadInbound: hadInbound}
						var admits bool
						if err := e.owner.QueryRow(context.Background(), `SELECT coalesce(`+mayAnswerSQL("$5::uuid", "a")+`, false)
							FROM (VALUES (NULLIF($1, ''), $2::text, $3::text, $4::text, $6::timestamptz))
							  AS a(direction, kind, source, captured_by, occurred_at)`,
							direction, kind, source, capturedBy, lead, date.AddDate(0, 0, 1)).Scan(&admits); err != nil {
							t.Fatal(err)
						}
						if accepts := isFirstResponseActivity(touch); accepts != admits {
							t.Errorf("mayAnswerSQL says %v for %+v, isFirstResponseActivity %v", admits, touch, accepts)
						} else if accepts {
							accepted++
						} else {
							refused++
						}
					}
				}
			}
		}
	}
	if accepted == 0 || refused == 0 {
		t.Fatalf("the grid holds %d accepted and %d refused touches; it must hold both", accepted, refused)
	}
}
