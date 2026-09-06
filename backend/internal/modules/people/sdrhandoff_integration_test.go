// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The SDR-to-AE handoff against a real database.
//
// Nothing in the unit lane reaches any of it. Every rule that matters here is a
// CHECK constraint or a WHERE clause the database evaluates: exactly one
// subject, a refusal that names its reason, a decision that happens once. Each
// can be wrong in a way that compiles and quietly records the wrong thing about
// somebody's work.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A handoff is submitted, accepted, and carries the deal the acceptance made.
//
// The deal id is the load-bearing half. It is what the held-meeting conversion
// anchors on, so that credit rests on the acceptance rather than on a deal that
// merely shares a company — an inference whose error grows with account size,
// always in the direction that flatters the SDR.
func TestAHandoffIsAcceptedWithTheDealItProduced(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "handoff-accept@example.test")
	deal := e.seedDealForHandoff(t)

	id, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{
		LeadID: leadPtr(lead), AssignedTo: &e.user, Note: "asked for pricing twice",
	})
	if err != nil {
		t.Fatalf("submitting the handoff: %v", err)
	}

	if err := e.store.DecideHandoff(e.ctx, id, HandoffDecision{
		Status: HandoffAccepted, DealID: &deal,
	}); err != nil {
		t.Fatalf("accepting the handoff: %v", err)
	}

	got := e.readHandoff(t, id)
	if got.status != HandoffAccepted {
		t.Errorf("status = %q, want accepted", got.status)
	}
	if got.dealID == nil || *got.dealID != deal {
		t.Errorf("deal = %v, want the deal the acceptance linked (%v)", got.dealID, deal)
	}
	if got.decidedAt == nil {
		t.Error("an accepted handoff carries no decided_at — nothing can age or order it")
	}

	// Both transitions survive, in order. The row says where it stands; the
	// events are the only place that says it was ever merely submitted.
	events := e.handoffEvents(t, id)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (submitted then accepted): %v", len(events), events)
	}
	if events[0] != HandoffSubmitted || events[1] != HandoffAccepted {
		t.Errorf("events = %v, want [submitted accepted]", events)
	}
}

// A refusal NAMES its reason, and the database is what makes that true.
//
// A reason nobody can count is a reason nobody acts on, which is the whole
// point of the administered catalog. The Go check gives the caller a sentence;
// this proves the constraint behind it, so a future writer that forgot cannot
// record an unexplained refusal.
func TestARejectedHandoffMustNameItsReason(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "handoff-reject@example.test")
	id, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{LeadID: leadPtr(lead)})
	if err != nil {
		t.Fatalf("submitting the handoff: %v", err)
	}

	err = e.store.DecideHandoff(e.ctx, id, HandoffDecision{Status: HandoffRejected})

	if !errors.Is(err, errHandoffNeedsReason) {
		t.Fatalf("rejecting with no reason: err = %v, want the reason to be required", err)
	}
	// And nothing was written: a refused decision leaves the handoff open for
	// somebody to decide properly.
	if got := e.readHandoff(t, id); got.status != HandoffSubmitted {
		t.Errorf("status = %q after a refused decision, want it still open", got.status)
	}
}

// A rejection with a reason lands, and the reason comes from the workspace's
// own list.
func TestARejectionRecordsTheReasonItNames(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "handoff-reason@example.test")
	reason := e.someHandoffReason(t, "rejected")
	id, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{LeadID: leadPtr(lead)})
	if err != nil {
		t.Fatalf("submitting the handoff: %v", err)
	}

	if err := e.store.DecideHandoff(e.ctx, id, HandoffDecision{
		Status: HandoffRejected, ReasonID: &reason, Note: "no budget named",
	}); err != nil {
		t.Fatalf("rejecting the handoff: %v", err)
	}

	got := e.readHandoff(t, id)
	if got.status != HandoffRejected {
		t.Errorf("status = %q, want rejected", got.status)
	}
	if got.reasonID == nil || *got.reasonID != reason {
		t.Errorf("reason = %v, want the one the decider named (%v)", got.reasonID, reason)
	}
	// The event carries the reason too. Reading it off the handoff alone would
	// answer "why is it rejected now" and not "why was it rejected then", which
	// is the question a recycled-and-rejected-again handoff makes different.
	if r := e.eventReason(t, id, HandoffRejected); r == nil || *r != reason {
		t.Errorf("the rejection event's reason = %v, want %v", r, reason)
	}
}

// A handoff is decided ONCE. A second decision is refused rather than
// overwriting the first, because two decisions on one handoff is two stories
// about what happened to one prospect.
func TestAHandoffIsDecidedOnlyOnce(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "handoff-twice@example.test")
	reason := e.someHandoffReason(t, "rejected")
	id, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{LeadID: leadPtr(lead)})
	if err != nil {
		t.Fatalf("submitting the handoff: %v", err)
	}
	if err := e.store.DecideHandoff(e.ctx, id, HandoffDecision{
		Status: HandoffRejected, ReasonID: &reason,
	}); err != nil {
		t.Fatalf("first decision: %v", err)
	}

	err = e.store.DecideHandoff(e.ctx, id, HandoffDecision{Status: HandoffAccepted})

	if !errors.Is(err, errHandoffAlreadyDecided) {
		t.Fatalf("second decision: err = %v, want it refused as already decided", err)
	}
	// The refusal says ALREADY DECIDED and not NOT FOUND. The two read
	// identically from RowsAffected and mean opposite things: one sends an AE
	// looking for a record that is on their screen.
	if errors.Is(err, apperrors.ErrNotFound) {
		t.Error("a decided handoff reported as missing — the AE is looking at it")
	}
	if got := e.readHandoff(t, id); got.status != HandoffRejected {
		t.Errorf("status = %q, want the first decision to stand", got.status)
	}
}

// A handoff names exactly ONE subject. Both, or neither, is two handoffs
// wearing one id or none at all.
func TestAHandoffNamesExactlyOneSubject(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "handoff-subject@example.test")
	person := e.seedPersonForHandoff(t)

	for _, tc := range []struct {
		name string
		in   NewSDRHandoff
	}{
		{"neither", NewSDRHandoff{}},
		{"both", NewSDRHandoff{LeadID: leadPtr(lead), PersonID: &person}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.store.SubmitHandoff(e.ctx, tc.in); !errors.Is(err, errHandoffNeedsOneSubject) {
				t.Errorf("err = %v, want exactly one subject to be required", err)
			}
		})
	}

	// And the positive case beside them, so the two refusals are not the whole
	// of what this test proves: a handoff with one subject is accepted.
	if _, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{PersonID: &person}); err != nil {
		t.Errorf("a handoff naming one person was refused: %v", err)
	}
}

// A refused handoff carries no deal. Attaching one would credit the submitter
// for work their handoff was refused for.
func TestARefusedHandoffCarriesNoDeal(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "handoff-nodeal@example.test")
	reason := e.someHandoffReason(t, "rejected")
	deal := e.seedDealForHandoff(t)
	id, err := e.store.SubmitHandoff(e.ctx, NewSDRHandoff{LeadID: leadPtr(lead)})
	if err != nil {
		t.Fatalf("submitting the handoff: %v", err)
	}

	err = e.store.DecideHandoff(e.ctx, id, HandoffDecision{
		Status: HandoffRejected, ReasonID: &reason, DealID: &deal,
	})

	if !errors.Is(err, errHandoffDealOnRefusal) {
		t.Fatalf("err = %v, want a deal on a refusal to be refused", err)
	}
	if got := e.readHandoff(t, id); got.dealID != nil {
		t.Errorf("the refused handoff carries deal %v", *got.dealID)
	}
}

type handoffRow struct {
	status    string
	reasonID  *ids.UUID
	dealID    *ids.UUID
	decidedAt *string
}

func (e *promoteConsentEnv) readHandoff(t *testing.T, id ids.UUID) handoffRow {
	t.Helper()
	var got handoffRow
	if err := e.owner.QueryRow(context.Background(), `
		SELECT status, reason_id, deal_id, decided_at::text FROM sdr_handoff WHERE id = $1`, id).
		Scan(&got.status, &got.reasonID, &got.dealID, &got.decidedAt); err != nil {
		t.Fatalf("reading the handoff: %v", err)
	}
	return got
}

func (e *promoteConsentEnv) handoffEvents(t *testing.T, id ids.UUID) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT to_status FROM sdr_handoff_event WHERE handoff_id = $1 ORDER BY occurred_at, id`, id)
	if err != nil {
		t.Fatalf("reading the handoff events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatalf("scanning a handoff event: %v", err)
		}
		out = append(out, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the handoff events: %v", err)
	}
	return out
}

func (e *promoteConsentEnv) eventReason(t *testing.T, id ids.UUID, status string) *ids.UUID {
	t.Helper()
	var reason *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT reason_id FROM sdr_handoff_event WHERE handoff_id = $1 AND to_status = $2`,
		id, status).Scan(&reason); err != nil {
		t.Fatalf("reading the event's reason: %v", err)
	}
	return reason
}

// someHandoffReason writes a reason for the transition it applies to.
//
// It seeds rather than reading the migration's own rows, because the harness
// resets every table before each test — a reader here would be asserting that
// truncation had not happened, which is a fact about the harness rather than
// about handoffs. That the migration seeds a usable list is a different claim,
// and TestTheSeededHandoffReasonsCoverBothTransitions is where it is made.
func (e *promoteConsentEnv) someHandoffReason(t *testing.T, appliesTo string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO sdr_handoff_reason (id, label, applies_to) VALUES ($1, $2, $3)`,
		id, "Seeded "+appliesTo+" reason", appliesTo); err != nil {
		t.Fatalf("seeding a %s reason: %v", appliesTo, err)
	}
	return id
}

// seedDealForHandoff writes the pipeline and stage a deal needs as well as the
// deal, because the harness resets every table before each test — reading a
// seeded pipeline here would assert that truncation had not happened.
func (e *promoteConsentEnv) seedDealForHandoff(t *testing.T) ids.UUID {
	t.Helper()
	pipeline, stage := ids.NewV7(), ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO pipeline (id, name) VALUES ($1, 'Handoff pipeline')`, pipeline); err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO stage (id, pipeline_id, name, position) VALUES ($1, $2, 'Qualify', 10)`,
		stage, pipeline); err != nil {
		t.Fatalf("seeding the stage: %v", err)
	}
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Handoff deal', $2, $3, 500000, 'EUR', 'manual', 'human:test')`,
		id, pipeline, stage); err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	return id
}

func (e *promoteConsentEnv) seedPersonForHandoff(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Handoff Subject', 'manual', 'human:test')`, id); err != nil {
		t.Fatalf("seeding the person: %v", err)
	}
	return id
}

// leadPtr hands the store a lead's plain id. The typed LeadID embeds it, so
// this reads the field rather than converting — the type exists to stop a person
// id being passed where a lead id belongs, and a cast would defeat it.
func leadPtr(id ids.LeadID) *ids.UUID {
	out := id.UUID
	return &out
}
