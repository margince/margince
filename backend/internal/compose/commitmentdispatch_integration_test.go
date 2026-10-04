// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the commitment rule remembers between readings of one conversation:
// a task a rep put away, and a customer's promise a rep said was never one.

import (
	"testing"

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

	e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
	if n := e.taskCount(t); n != 0 {
		t.Errorf("a refused promise became %d task(s) on a surer reading", n)
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
