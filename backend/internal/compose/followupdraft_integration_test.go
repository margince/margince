// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The nightly pass's drafted reply, over real migrated Postgres.
//
// What these hold is the claim the design rests on: held_draft's release path
// was written for an automation that parked a workflow run, and the overnight
// pass parks none. If completing that run were required rather than
// conditional, every drafted follow-up would fail at release — after a human
// had already approved it, which is the worst place to find out.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/automation"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// seedAnswerableThread plants the inbound email a reply can be drafted to: a
// person with an address, that address on the thread, and the consent purpose
// the send is gated against. It returns the anchor activity.
// The address is derived from the subject so two threads on one deal are two
// counterparties: person_email is unique per address, and a helper that always
// seeded the same one could only ever be called once per test.
func (e *reconcileEnv) seedAnswerableThread(t *testing.T, dealID ids.UUID, subject string) ids.UUID {
	t.Helper()
	to := strings.ToLower(strings.ReplaceAll(subject, " ", ".")) + "@example.test"
	person := e.SeedPerson(t, "Anna Weber ("+subject+")", nil)
	anchor := e.seedInteraction(t, dealID, "email", subject, 1)
	e.WsExec(t, `UPDATE activity SET direction = 'inbound' WHERE id = $1`, anchor)
	e.attachReachableCounterpartyAs(t, anchor, person, to)
	return anchor
}

// attachReachableCounterparty puts a person with a real address on the
// activity, which is what makes a reply address resolvable.
func (e *reconcileEnv) attachReachableCounterparty(t *testing.T, activityID ids.UUID, address string) {
	t.Helper()
	e.attachReachableCounterpartyAs(t, activityID, e.SeedPerson(t, "Counterparty "+address, nil), address)
}

func (e *reconcileEnv) attachReachableCounterpartyAs(
	t *testing.T, activityID, person ids.UUID, address string,
) {
	t.Helper()
	// The purpose the send is gated against. Its class is the one that is never
	// consent-gated: answering somebody who wrote to you rests on contract or
	// legitimate interest, not on a consent record.
	e.WsExec(t, `
		INSERT INTO consent_purpose (key, label, requires_double_opt_in, class)
		VALUES ('business_correspondence', 'Business correspondence', false, 'business_correspondence')
		ON CONFLICT (key) DO NOTHING`)
	e.WsExec(t, `
		INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'test', 'human:seed')`, person, address)
	e.WsExec(t, `
		INSERT INTO activity_participant (id, activity_id, role, person_id, address)
		VALUES ($1, $2, 'from', $3, $4)`, ids.NewV7(), activityID, person, address)
	e.WsExec(t, `
		INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), activityID, person)
}

// heldDraftFor reads the drafted reply the pass staged on this deal.
func (e *reconcileEnv) heldDraftFor(t *testing.T, dealID ids.UUID) (ids.ApprovalID, automation.HeldDraftProposal) {
	t.Helper()
	var id ids.ApprovalID
	var raw []byte
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id, proposed_change FROM approval
		 WHERE kind = 'held_draft' AND target_entity_id = $1 AND status = 'pending'`,
		dealID).Scan(&id, &raw); err != nil {
		t.Fatalf("no drafted reply staged on the deal: %v", err)
	}
	var proposal automation.HeldDraftProposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		t.Fatalf("the staged draft does not decode: %v", err)
	}
	return id, proposal
}

// An email thread earns the reply itself, not a task telling a rep to write one.
func TestAnEmailThreadWithNoNextStepStagesADraftedReply(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Answerable", e.pipeline, e.open, &e.Rep1)
	anchor := e.seedAnswerableThread(t, deal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}

	_, draft := e.heldDraftFor(t, deal)
	if draft.AnchorActivityID != anchor {
		t.Errorf("the draft answers activity %s, want the thread %s", draft.AnchorActivityID, anchor)
	}
	if draft.To == "" {
		t.Error("the draft has no recipient — a draft that hides who it is to cannot be approved meaningfully")
	}
	if draft.Body == "" || draft.Subject == "" {
		t.Errorf("the draft is empty (subject %q, body %q), want words the rep can read and send",
			draft.Subject, draft.Body)
	}
	if draft.ConsentPurpose != followUpDraftPurpose {
		t.Errorf("consent purpose = %q, want %q — a send with no lawful basis is refused at the gate",
			draft.ConsentPurpose, followUpDraftPurpose)
	}
	// And NOT the task proposal beside it: one conversation, one question.
	if got := e.pendingFollowUps(t, deal); got != 0 {
		t.Errorf("the pass staged %d task proposals beside the draft, want 0 — "+
			"a rep asked twice about one conversation answers neither", got)
	}
}

// A call has no thread to answer, so it keeps the task proposal.
func TestACallWithNoNextStepStillStagesTheTaskProposal(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Unanswerable", e.pipeline, e.open, &e.Rep1)
	// A counterparty WITH a reachable address, so what keeps this on the task
	// proposal is the kind rule and nothing else. Without the address the
	// unanswerable-thread fallback would carry the test, and dropping the kind
	// check entirely would not fail it.
	call := e.seedInteraction(t, deal, "call", "Discovery call", 1)
	e.attachReachableCounterparty(t, call, "caller@example.test")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, deal); got != 1 {
		t.Errorf("task proposals on a call-evidenced deal = %d, want 1", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval
		WHERE kind = 'held_draft' AND target_entity_id = $1`, deal); n != 0 {
		t.Errorf("the pass drafted %d replies to a call, want 0 — there is no thread to answer", n)
	}
}

// An email thread whose counterparty cannot be reached falls back to the task.
//
// The rep is still told about the deal. Dropping the candidate would be the
// tempting alternative and the wrong one: the deal genuinely has no next step,
// which is the thing worth saying, and the missing address only decides HOW it
// can be answered.
func TestAThreadWithNoReplyAddressFallsBackToTheTaskProposal(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "No address", e.pipeline, e.open, &e.Rep1)
	// An inbound email with no participant and no person behind it.
	anchor := e.seedInteraction(t, deal, "email", "From a stranger", 1)
	e.WsExec(t, `UPDATE activity SET direction = 'inbound' WHERE id = $1`, anchor)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, deal); got != 1 {
		t.Errorf("task proposals = %d, want 1 — an unanswerable thread must still "+
			"tell the rep the deal has no next step", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval
		WHERE kind = 'held_draft' AND target_entity_id = $1`, deal); n != 0 {
		t.Errorf("the pass staged %d drafts with no address to send to, want 0", n)
	}
}

// A rejected draft is not redrafted the next night.
func TestARejectedDraftedReplyIsNotOfferedAgain(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Declined draft", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, deal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.heldDraftFor(t, deal)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval
		WHERE kind = 'held_draft' AND target_entity_id = $1 AND status = 'pending'`, deal); n != 0 {
		t.Errorf("a declined draft came back as %d pending offers, want 0", n)
	}
}

// The identity is the THREAD, so a later conversation is drafted again.
func TestANewThreadIsDraftedAfterAnEarlierDraftWasDeclined(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Wrote again", e.pipeline, e.open, &e.Rep1)
	declined := e.seedAnswerableThread(t, deal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.heldDraftFor(t, deal)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}

	fresh := e.seedAnswerableThread(t, deal, "They wrote again")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	_, draft := e.heldDraftFor(t, deal)
	if draft.AnchorActivityID != fresh {
		t.Errorf("the new draft answers %s, want the fresh thread %s", draft.AnchorActivityID, fresh)
	}
	if draft.AnchorActivityID == declined {
		t.Error("the new draft answers the thread whose reply was already declined")
	}
}

// The claim the whole design rests on: the release path completes a parked
// workflow run, and the overnight pass parks none.
//
// If that completion were required rather than conditional, this send would
// fail AFTER a human approved it — the one place a failure cannot be taken
// back. The assertion is that the mail goes out and the approval is spent.
func TestADraftedFollowUpReleasesWithNoWorkflowRunBehindIt(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Send it", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, deal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.heldDraftFor(t, deal)

	// The release path as production binds it: registered LATE, over a
	// configured send path. A bare one would refuse for reasons that have
	// nothing to do with the question here.
	releaser := releaseService(t, e.Env)
	if _, err := releaser.Decide(decider(e.Env), approvalID, true, nil); err != nil {
		t.Fatalf("releasing a drafted follow-up → %v, want the send to succeed", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM activity
		WHERE direction = 'outbound' AND kind = 'email'`); n != 1 {
		t.Errorf("outbound emails = %d, want exactly 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM comms_outbound`); n != 1 {
		t.Errorf("delivery rows = %d, want exactly 1 — the activity and its delivery are one fact", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval
		WHERE id = $1 AND consumed_at IS NOT NULL`, approvalID); n != 1 {
		t.Error("the approval was not consumed — a send whose authority is still redeemable can be sent twice")
	}
}

// A drafted reply is a held_draft, and the pass may not invent a second kind
// for the same question.
func TestTheDraftedFollowUpUsesTheExistingHeldDraftKind(t *testing.T) {
	if automation.HeldDraftKind == deals.FollowUpReconcileKind {
		t.Fatal("the two proposal kinds collapsed into one, so this test proves nothing")
	}
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "One kind", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, deal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	rows, err := e.owner.Query(context.Background(),
		`SELECT DISTINCT kind FROM approval WHERE target_entity_id = $1`, deal)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 1 || kinds[0] != automation.HeldDraftKind {
		t.Errorf("the pass staged kinds %v, want exactly [%s] — a second drafted-email "+
			"kind is a second send path and a second consent call",
			kinds, automation.HeldDraftKind)
	}
}
