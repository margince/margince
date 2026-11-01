// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The week's leads are one cohort, and its size is frozen. Two leads arrive
// and both breach. One is answered late, the other never. A third arrived and
// was archived. So two arrived, one has a recorded response, two breached, and
// one has no response. The archived lead is in none of these figures.
func TestTheWeeksLeadsAreOneCohortAndItsSizeIsFrozen(t *testing.T) {
	e := setupWeekly(t)
	owner := ids.From[ids.UserKind](e.Rep1)
	var arrivedAt time.Time
	create := func(name string) ids.LeadID {
		t.Helper()
		lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
			FullName: &name, Status: "new", OwnerID: &owner, Source: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		if arrivedAt.IsZero() {
			arrivedAt = lead.CreatedAt
		}
		return ids.From[ids.LeadKind](ids.UUID(lead.Id))
	}
	lateAnswered := create("Answered late")
	neverAnswered := create("Never answered")
	archived := create("Archived the same day")

	start, end := arrivedAt.Add(-time.Hour), arrivedAt.Add(72*time.Hour)

	// The breach stamp is set directly, as the imported-history test does. The
	// native sweep stamps only a lead that owes a reply to an inbound inquiry.
	// This test is about how the week counts stamps, not when they are written.
	for _, id := range []ids.LeadID{lateAnswered, neverAnswered, archived} {
		e.WsExec(t, `UPDATE lead SET sla_breached_at = $2 WHERE id = $1`, id.UUID, arrivedAt.Add(30*time.Minute))
	}
	for _, id := range []ids.LeadID{lateAnswered, archived} {
		if wrote, err := e.Contacts.RecordLeadFirstResponse(e.Admin(), id, arrivedAt.Add(49*time.Hour)); err != nil || !wrote {
			t.Fatalf("record late response: %v, wrote=%v", err, wrote)
		}
	}
	// No public lead-archive command exists in this module; the archived state
	// is the input under test, so it is set directly.
	e.WsExec(t, `UPDATE lead SET archived_at = $2 WHERE id = $1`, archived.UUID, arrivedAt.Add(time.Hour))

	err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		routed, answered, breached, err := countWeekLeads(e.repCtx, tx, e.Rep1, start, end)
		if err != nil {
			return err
		}
		if routed != 2 || answered != 1 || breached != 2 {
			t.Errorf("weekly counts = arrived %d, responded %d, breached %d; want 2, 1, 2", routed, answered, breached)
		}
		card, err := scoreLeads(e.repCtx, tx, e.Rep1, start, end)
		if err != nil {
			return err
		}
		if card == nil || card.Arrived == nil {
			t.Fatalf("a newly scored week must record its cohort, got %+v", card)
		}
		if *card.Arrived != 2 || card.AnsweredInTarget != 1 || card.Breached != 2 {
			t.Errorf("scorecard = arrived %d, responded %d, breached %d; want 2, 1, 2",
				*card.Arrived, card.AnsweredInTarget, card.Breached)
		}
		if unanswered := *card.Arrived - card.AnsweredInTarget; unanswered != 1 {
			t.Errorf("leads with no recorded first response = %d, want 1", unanswered)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A week frozen before the cohort was recorded reads it as unknown. Zero would
// tell a reader no lead went unanswered that week.
func TestAScorecardFrozenWithoutItsCohortReadsItAsUnknown(t *testing.T) {
	e := setupWeekly(t)
	seedLeadFor(t, e, "Cohort freeze", e.Rep1)
	written, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	if written.Scorecard == nil || written.Scorecard.Lead == nil || written.Scorecard.Lead.Arrived == nil {
		t.Fatalf("a freshly frozen lead block must carry its cohort, got %+v", written.Scorecard)
	}
	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.Scorecard.Lead.Arrived == nil || *read.Scorecard.Lead.Arrived != *written.Scorecard.Lead.Arrived {
		t.Fatalf("the frozen cohort must read back unchanged, wrote %v read %v",
			*written.Scorecard.Lead.Arrived, read.Scorecard.Lead.Arrived)
	}
	if wire := reviewToWire(read); wire.Scorecard.Lead.Arrived == nil {
		t.Fatal("the cohort must reach the wire")
	}

	// The shape of a row written before the column existed.
	e.WsExec(t, `UPDATE weekly_review_scorecard SET lead_arrived = NULL WHERE weekly_review_id = $1`, written.ID)
	legacy, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Scorecard.Lead.Arrived != nil {
		t.Fatalf("a legacy row's cohort is unknown, not %d", *legacy.Scorecard.Lead.Arrived)
	}
	if wire := reviewToWire(legacy); wire.Scorecard.Lead.Arrived != nil {
		t.Fatal("an unknown cohort must not travel as a number")
	}
}

// A manager's team week freezes a lead answered late. The lead is in the
// routed, answered and breached tallies. The team snapshot used to require
// answered + breached <= routed, and refused to freeze such a week.
func TestATeamWeekHoldingALateAnswerFreezes(t *testing.T) {
	e := setupWeekly(t)
	owner := ids.From[ids.UserKind](e.Rep1)
	name := "Answered late, team week"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, Status: "new", OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Mid-week, so the breach and the answer fall in the lead's own week at
	// whatever hour the suite runs. A lead made late on a Sunday was otherwise
	// answered in the next week.
	arrived := midWeek(clocktest.Now(t))
	e.WsExec(t, `UPDATE lead SET created_at = $2, sla_breached_at = $3 WHERE id = $1`,
		lead.Id, arrived, arrived.Add(30*time.Minute))
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if wrote, err := e.Contacts.RecordLeadFirstResponse(e.Admin(), leadID, arrived.Add(time.Hour)); err != nil || !wrote {
		t.Fatalf("record late response: %v, wrote=%v", err, wrote)
	}

	at := arrived.AddDate(0, 0, 7)
	if _, _, err := e.engine.AssembleFor(e.repCtx, at); err != nil {
		t.Fatal(err)
	}
	members := []TeamMember{{UserID: e.Rep1, DisplayName: "Rep One"}}
	team, _, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Team One", members, at)
	if err != nil {
		t.Fatalf("a team week holding a late answer must freeze: %v", err)
	}
	c := team.Counts
	if c.LeadsRouted != 1 || c.LeadsAnsweredInTarget != 1 || c.LeadsBreached != 1 {
		t.Fatalf("team week = routed %d, responded %d, breached %d; want 1, 1, 1",
			c.LeadsRouted, c.LeadsAnsweredInTarget, c.LeadsBreached)
	}
}

// midWeek is noon on the Thursday of the UTC week holding at.
func midWeek(at time.Time) time.Time {
	at = at.UTC()
	monday := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -((int(at.Weekday()) + 6) % 7))
	return monday.AddDate(0, 0, 3).Add(12 * time.Hour)
}
