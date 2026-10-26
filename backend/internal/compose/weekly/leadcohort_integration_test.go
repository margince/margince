// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// THE WEEK'S LEADS ARE ONE COHORT, AND ITS SIZE IS FROZEN.
//
// Two leads arrive. Nobody answers either in time, so both breach. One is then answered late, the other never. A third lead
// arrived the same day and was archived. The expected figures follow from that
// story, not from the queries: two arrived, one has a first response recorded
// by the close (late, so it is also a breach), two breached, and one has no
// recorded response. The archived lead is in none of them.
//
// Before this, the scorecard counted response and breach over every lead the
// rep owned, archived or not, and froze no cohort at all, so no reader could
// tell "one of two never answered" from "both answered".
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

	// The breach stamp is set directly, as the imported-history test beside
	// this one does. The native sweep stamps only a lead that owes a reply to
	// an inbound inquiry, and this test is about how the week counts stamps,
	// not about when the sweep writes them.
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

// A WEEK FROZEN BEFORE THE COHORT WAS RECORDED READS IT AS UNKNOWN.
//
// The column is NULL on every scorecard written before it existed. Reading that
// as zero would tell a reader no lead went unanswered that week.
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
