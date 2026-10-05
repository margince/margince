// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the commitment rule remembers between readings of one conversation:
// a task a rep put away, and a customer's promise a rep said was never one.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A task a rep archived stays archived: reading the meeting again neither
// writes it again nor asks about it.
func TestAnArchivedCommitmentTaskIsNotWrittenAgain(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.9)})
	task := e.wsString(t, `SELECT id::text FROM activity WHERE kind = 'task'`)
	taskID, err := ids.Parse(task)
	if err != nil {
		t.Fatalf("reading the task id: %v", err)
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](taskID), nil); err != nil {
		t.Fatalf("archiving the task: %v", err)
	}

	again := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.9)})
	if len(again.ProposalIDs) != 0 {
		t.Errorf("the archived promise was put to a human again (%d proposals)", len(again.ProposalIDs))
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 1 {
		t.Errorf("reading the meeting again left %d task rows, want the one archived task", n)
	}
}

// A reading that words the promise differently still finds it: the task is
// keyed on who promised and the lines it was said on, not on the summary.
func TestARewordedPromiseIsTheSamePromise(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.9)})
	e.read(t, cannedBrain{reply: reworded(t, "Get the new price list to them", 3)})
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 1 {
		t.Errorf("a reworded reading of one promise left %d tasks", n)
	}
}

// A customer's promise a rep dismissed stays dismissed, and is not filed again
// beside the dismissed one.
func TestADismissedCustomerPromiseIsNotFiledAgain(t *testing.T) {
	e := setupTranscript(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Frédéric de Gombert"})
	if err != nil {
		t.Fatalf("creating the customer: %v", err)
	}
	integration.LinkActivity(t, e.owner, e.activity.UUID, "contact", ids.UUID(contact.Id))
	e.read(t, cannedBrain{reply: ownedReply(t, "Frédéric de Gombert", 0.9)})
	claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
	if err != nil {
		t.Fatalf("reading the claim id: %v", err)
	}
	if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, "dismissed"); err != nil {
		t.Fatalf("dismissing the claim: %v", err)
	}

	e.read(t, cannedBrain{reply: ownedReply(t, "Frédéric de Gombert", 0.9)})
	got := e.wsString(t, `SELECT count(*)::text || ' ' || string_agg(status, ',') FROM conversation_claim
		WHERE archived_at IS NULL`)
	if got != "1 dismissed" {
		t.Errorf("after a second reading the claims read %q, want the one dismissed claim", got)
	}
}

// A promise a human turned down as a proposal is not written as a task when a
// later reading of the same words is surer of it.
func TestARefusedPromiseIsNotWrittenWhenAReadingIsSurer(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	read := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.75)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("want the unsure promise proposed, got %d proposals", len(read.ProposalIDs))
	}
	priya := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	reason := "not mine"
	if _, err := e.svc.Decide(priya, ids.From[ids.ApprovalKind](read.ProposalIDs[0]), false, &reason); err != nil {
		t.Fatalf("refusing the proposal: %v", err)
	}

	again := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
	if n := e.taskCount(t); n != 0 || len(again.ProposalIDs) != 0 {
		t.Errorf("a refused promise became %d task(s) and %d proposal(s) on a surer reading",
			n, len(again.ProposalIDs))
	}
}

// A promise still waiting on a human is not written around them when a later
// reading of the same words is surer of it.
func TestAWaitingPromiseIsNotWrittenWhenAReadingIsSurer(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	if read := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.75)}); len(read.ProposalIDs) != 1 {
		t.Fatalf("want the unsure promise proposed, got %d proposals", len(read.ProposalIDs))
	}
	again := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
	if n := e.taskCount(t); n != 0 || len(again.ProposalIDs) != 0 {
		t.Errorf("a waiting promise became %d task(s) and %d new proposal(s) on a surer reading",
			n, len(again.ProposalIDs))
	}
}

// A customer who shares a colleague's exact name is not taken for the
// colleague: the promise is put to a human rather than written as our task.
func TestACustomerSharingAColleaguesNameIsNotTakenForThem(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Priya Raman"})
	if err != nil {
		t.Fatalf("creating the customer: %v", err)
	}
	integration.LinkActivity(t, e.owner, e.activity.UUID, "contact", ids.UUID(contact.Id))

	read := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
	if e.taskCount(t) != 0 || len(read.ProposalIDs) != 1 {
		t.Errorf("a name both sides answer to became %d task(s) and %d proposal(s); want one proposal",
			e.taskCount(t), len(read.ProposalIDs))
	}
}

// linkInes files the meeting under a customer called Ines Huber, who a promise
// of ours in it was made to when she is its only customer.
func linkInes(t *testing.T, e *transcriptEnv) ids.UUID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Ines Huber"})
	if err != nil {
		t.Fatalf("creating the customer: %v", err)
	}
	integration.LinkActivity(t, e.owner, e.activity.UUID, "contact", ids.UUID(contact.Id))
	return ids.UUID(contact.Id)
}

// A promise of ours in a meeting with one customer is filed on that customer
// as well, dated as the meeting said, and the claim points at the task it
// became — so their record shows the promise and opens the task.
func TestOurPromiseIsFiledOnTheCustomerItWasMadeTo(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	customer := linkInes(t, e)

	raw, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": "Send the revised pricing", "owner": "Priya Raman",
		"due_date": "2026-09-08", "source_lines": []int{3}, "confidence": 0.9,
	}}})
	if err != nil {
		t.Fatalf("building the model reply: %v", err)
	}
	e.read(t, cannedBrain{reply: string(raw)})

	zone, err := installationZone(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("reading the installation's zone: %v", err)
	}
	got := e.wsString(t, `SELECT (c.task_activity_id = t.id)::text || ' ' || extract(epoch FROM c.due_at)::bigint::text
		FROM conversation_claim c, activity t
		WHERE c.kind = 'commitment_ours' AND c.contact_id = $1 AND t.kind = 'task'`, customer)
	if want := fmt.Sprintf("true %d", time.Date(2026, 9, 8, 23, 59, 59, 0, zone).Unix()); got != want {
		t.Errorf("the claim on the customer reads %q, want it dated and pointing at the task", got)
	}
}

// A promise of ours a rep dismissed from the customer's record is not proposed
// again when the meeting is read again.
func TestADismissedPromiseOfOursIsNotProposedAgain(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	linkInes(t, e)
	read := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.75)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("want the unsure promise proposed, got %d", len(read.ProposalIDs))
	}
	claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
	if err != nil {
		t.Fatalf("reading the claim: %v", err)
	}
	if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, "dismissed"); err != nil {
		t.Fatalf("dismissing the claim: %v", err)
	}
	priya := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	reason := "never said that"
	if _, err := e.svc.Decide(priya, ids.From[ids.ApprovalKind](read.ProposalIDs[0]), false, &reason); err != nil {
		t.Fatalf("refusing the proposal: %v", err)
	}

	again := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
	if len(again.ProposalIDs) != 0 || e.taskCount(t) != 0 {
		t.Errorf("a dismissed promise came back as %d proposal(s) and %d task(s)", len(again.ProposalIDs), e.taskCount(t))
	}
}

// Two customers of one name tie a promise to neither: it is put to a human.
func TestTwoCustomersOfOneNameTieThePromiseToNeither(t *testing.T) {
	e := setupTranscript(t)
	linkInes(t, e)
	linkInes(t, e)
	read := e.read(t, cannedBrain{reply: ownedReply(t, "Ines Huber", 0.95)})
	if len(read.ProposalIDs) != 1 {
		t.Errorf("a name two customers answer to became %d proposals, want one put to a human", len(read.ProposalIDs))
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE kind = 'commitment_theirs'`); n != 0 {
		t.Errorf("the promise was filed on %d customers who share the name", n)
	}
}

// A proposal whose source conversation is gone by the time a human accepts it
// is refused, not written as a task nobody can check.
func TestAProposalWhoseConversationIsGoneIsNotAccepted(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	read := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.75)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("want the unsure promise proposed, got %d proposals", len(read.ProposalIDs))
	}
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, e.activity.UUID)

	priya := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	if _, err := e.svc.Decide(priya, ids.From[ids.ApprovalKind](read.ProposalIDs[0]), true, nil); err == nil {
		t.Error("a proposal whose conversation was archived was accepted")
	}
	if n := e.taskCount(t); n != 0 {
		t.Errorf("accepting it wrote %d task(s)", n)
	}
}

// A card decided under the retired transcript_proposal kind stays readable to
// the rep it was staged for, though nothing stages or applies that kind now.
func TestADecidedRetiredTranscriptCardStaysReadable(t *testing.T) {
	e := setupTranscript(t)
	id := integration.SeedIDRow(t, e.owner, `INSERT INTO approval
		(id, kind, status, proposed_by, on_behalf_of, proposed_change, diff_hash, expires_at, decided_at,
		 target_entity_type, target_entity_id)
		VALUES ($1, 'transcript_proposal', 'approved', 'agent:transcript-proposer', $2, '{}', 'retired-1',
		        now() + interval '1 day', now(), 'activity', $3)`, e.Rep1, e.activity.UUID)
	if _, err := e.svc.Get(e.ctx, ids.From[ids.ApprovalKind](id)); err != nil {
		t.Errorf("the rep cannot open a card decided under the retired kind: %v", err)
	}
}
