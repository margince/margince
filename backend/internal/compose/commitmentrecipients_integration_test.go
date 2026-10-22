// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func recipientClaim(t *testing.T, k keptCommitment, owner ids.UUID) ids.UUID {
	t.Helper()
	recipient := k.SeedContact(t, "Morgan Recipient", &owner)

	quote := k.wsString(t, `SELECT source_quote FROM conversation_claim WHERE id=$1`, k.claim)
	locator := k.wsString(t, `SELECT evidence_fingerprint FROM conversation_claim WHERE id=$1`, k.claim)
	var claim crmcontracts.ConversationClaim
	err := InstallationDB(k.Pool).Tx(k.Admin(), func(tx pgx.Tx) error {
		var err error
		claim, _, err = k.Contacts.RecordConversationClaimTx(k.Admin(), tx, contacts.ClaimInput{
			ContactID: ids.From[ids.ContactKind](recipient), Kind: claimKindOurs,
			Body: "Send the agreed material", ActivityID: k.activity.UUID, Quote: quote,
			Source: "test", Locator: locator,
		})
		if err != nil {
			return err
		}
		return k.Contacts.SetClaimTaskTx(k.Admin(), tx, ids.UUID(claim.Id), k.task)
	})
	if err != nil {
		t.Fatal(err)
	}
	if owner == k.Rep3 {
		if _, err := k.Contacts.UpdateContact(recipientOwner(k, owner), ids.From[ids.ContactKind](recipient), contacts.UpdateContactInput{Visibility: new("owner")}); err != nil {
			t.Fatal(err)
		}
	}
	return ids.UUID(claim.Id)
}

func TestOneRecipientCannotCompleteAnotherRecipientsWork(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		name := "visible recipient"
		if hidden {
			name = "recipient outside the settler scope"
		}
		t.Run(name, func(t *testing.T) {
			k := seedKeptCommitment(t)
			owner := k.Rep2
			if hidden {
				owner = k.Rep3
			}
			other := recipientClaim(t, k, owner)
			actor := k.As(k.Rep2, []ids.UUID{k.Team1}, transcriptPerms)
			if err := k.Contacts.SettleConversationClaim(actor, k.claim, claimStatusDone); err != nil {
				t.Fatal(err)
			}
			first := k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, k.claim.String())
			k.deliver(t, first)
			k.deliver(t, first)
			if k.taskDone(t) != "false" {
				t.Fatal("one recipient completed the shared task")
			}
			if status := k.wsString(t, `SELECT status FROM conversation_claim WHERE id=$1`, other); status != "open" {
				t.Fatal("another recipient was settled by partial completion")
			}
			visible, err := (attentionTasks{claims: k.Contacts}).withRecipients(actor, []attention.Task{{ID: k.task}}, []ids.UUID{k.task})
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if hidden {
				want = 1
			}
			if len(visible[0].Recipients) != want {
				t.Fatalf("visible recipients=%d, want %d", len(visible[0].Recipients), want)
			}
			for _, recipient := range visible[0].Recipients {
				expected := "open"
				if ids.UUID(recipient.ClaimId) == k.claim {
					expected = "done"
				}
				if string(recipient.Status) != expected || recipient.DisplayName == "" {
					t.Fatalf("recipient state lost: %+v", recipient)
				}
			}
			if err := k.Contacts.SettleConversationClaim(recipientOwner(k, owner), other, claimStatusDone); err != nil {
				t.Fatal(err)
			}
			k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, other.String()))
			if hidden {
				if k.taskDone(t) != "false" {
					t.Fatal("recipient owner completed another colleague's task without write authority")
				}
				k.setDone(t, true)
			}
			if k.taskDone(t) != "true" {
				t.Fatal("all recipient obligations settled but task stayed open")
			}
		})
	}
}

func TestCompletedTaskCannotAcquireAnOpenRecipient(t *testing.T) {
	k := seedKeptCommitment(t)
	k.setDone(t, true)
	recipient := k.SeedContact(t, "Later Recipient", &k.Rep2)
	quote := k.wsString(t, `SELECT source_quote FROM conversation_claim WHERE id=$1`, k.claim)
	locator := k.wsString(t, `SELECT evidence_fingerprint FROM conversation_claim WHERE id=$1`, k.claim)
	err := InstallationDB(k.Pool).Tx(k.Admin(), func(tx pgx.Tx) error {
		claim, _, err := k.Contacts.RecordConversationClaimTx(k.Admin(), tx, contacts.ClaimInput{
			ContactID: ids.From[ids.ContactKind](recipient), Kind: claimKindOurs, Body: "Send the agreed material",
			ActivityID: k.activity.UUID, Quote: quote, Source: "test", Locator: locator,
		})
		if err != nil {
			return err
		}
		return k.Contacts.SetClaimTaskTx(k.Admin(), tx, ids.UUID(claim.Id), k.task)
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("linking an open claim to completed work: %v", err)
	}
	if count := k.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE contact_id=$1`, recipient); count != 0 {
		t.Fatal("failed recipient linking left an orphaned obligation")
	}
}

func TestPartialRecipientSettlementKeepsTheSharedProposal(t *testing.T) {
	e := setupCompletionTranscript(t, transcriptBody)
	linkInes(t, e)
	reading := e.readAt(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)}, func() time.Time { return completionReadTime })
	if len(reading.ProposalIDs) != 1 {
		t.Fatal("expected one proposed deliverable")
	}
	first, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
	if err != nil {
		t.Fatal(err)
	}
	quote := e.wsString(t, `SELECT source_quote FROM conversation_claim WHERE id=$1`, first)
	locator := e.wsString(t, `SELECT evidence_fingerprint FROM conversation_claim WHERE id=$1`, first)
	recipient := e.SeedContact(t, "Morgan Recipient", &e.Rep3)
	second, err := e.Contacts.RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](recipient), Kind: claimKindOurs, Body: "Send revised pricing",
		ActivityID: e.activity.UUID, Quote: quote, Source: "test", Locator: locator,
	})
	if err != nil {
		t.Fatal(err)
	}
	k := keptCommitment{transcriptEnv: e, claim: first, settle: NewCommitmentSettleTrigger(e.Pool, slog.Default())}
	for index, claim := range []ids.UUID{first, ids.UUID(second.Id)} {
		if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, claimStatusDone); err != nil {
			t.Fatal(err)
		}
		event := k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, claim.String())
		k.deliver(t, event)
		k.deliver(t, event)
		want := "pending"
		if index == 1 {
			want = "expired"
		}
		if status := e.wsString(t, `SELECT status FROM approval WHERE id=$1`, reading.ProposalIDs[0]); status != want {
			t.Fatalf("proposal after recipient %d: %s, want %s", index+1, status, want)
		}
	}
}

func recipientOwner(k keptCommitment, owner ids.UUID) context.Context {
	perms := transcriptPerms
	perms.RowScope = principal.RowScopeAll
	return k.As(owner, nil, perms)
}

func TestUnlinkedRecipientPreventsAutomaticTaskCompletion(t *testing.T) {
	k := seedKeptCommitment(t)
	recipient := k.SeedContact(t, "Recipient awaiting review", &k.Rep2)
	quote := k.wsString(t, `SELECT source_quote FROM conversation_claim WHERE id=$1`, k.claim)
	locator := k.wsString(t, `SELECT evidence_fingerprint FROM conversation_claim WHERE id=$1`, k.claim)
	other, err := k.Contacts.RecordConversationClaim(k.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](recipient), Kind: claimKindOurs,
		Body: "Send the agreed material", ActivityID: k.activity.UUID, Quote: quote,
		Source: "test", Locator: locator,
	})
	if err != nil {
		t.Fatal(err)
	}
	actor := k.As(k.Rep2, []ids.UUID{k.Team1}, transcriptPerms)
	if err := k.Contacts.SettleConversationClaim(actor, k.claim, claimStatusDone); err != nil {
		t.Fatal(err)
	}
	k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, k.claim.String()))
	if k.taskDone(t) != "false" {
		t.Fatal("unlinked recipient was omitted from automatic completion")
	}
	if err := InstallationDB(k.Pool).Tx(actor, func(tx pgx.Tx) error {
		return k.Contacts.SetClaimTaskTx(actor, tx, ids.UUID(other.Id), k.task)
	}); err != nil {
		t.Fatal(err)
	}
	if err := k.Contacts.SettleConversationClaim(actor, ids.UUID(other.Id), claimStatusDone); err != nil {
		t.Fatal(err)
	}
	k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, other.Id.String()))
	if k.taskDone(t) != "true" {
		t.Fatal("fully settled recipients left the task open")
	}
}

// A promise request about a task follows the task's current assignee. A human
// hands the task on, and a re-read finds another recipient. The request put to
// the former assignee is withdrawn, and only the new one is asked.
func TestARequestAboutAReassignedTaskLeavesTheFormerAssignee(t *testing.T) {
	e, readFor, handTo := setupRecipientTaskTranscript(t)
	pendingFor := func(seat ids.UUID) int {
		t.Helper()
		return e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task' AND status = 'pending'
			AND requested_decider_id = $1`, seat)
	}

	readFor(1)
	handTo(e.Rep3)
	readFor(2)
	if pendingFor(e.Rep3) != 1 {
		t.Fatalf("want the new recipient put to the task's holder; got %d request(s)", pendingFor(e.Rep3))
	}
	handTo(e.Rep1)
	readFor(3)

	if n := pendingFor(e.Rep3); n != 0 {
		t.Errorf("%d request(s) still put to the former assignee, want none", n)
	}
	if n := pendingFor(e.Rep1); n != 1 {
		t.Errorf("%d request(s) put to the current assignee, want 1", n)
	}
}

// A task held by an archived colleague takes no new recipients by request.
// Nobody else is asked in their place: not the promiser, the stager or the
// record's owner.
func TestARequestAboutATaskAnArchivedColleagueHoldsAsksNobody(t *testing.T) {
	e, readFor, handTo := setupRecipientTaskTranscript(t)
	readFor(1)
	handTo(e.Rep3)
	e.WsExec(t, `UPDATE app_user SET archived_at = now() WHERE id = $1`, e.Rep3)
	readFor(2)

	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task' AND status = 'pending'`); n != 0 {
		t.Errorf("%d request(s) staged about a task its archived holder cannot take, want none", n)
	}
}

// setupRecipientTaskTranscript reads a promise to three contacts. readFor reads
// it again naming the first n of them, and handTo hands its task to a seat.
func setupRecipientTaskTranscript(t *testing.T) (*transcriptEnv, func(int), func(ids.UUID)) {
	t.Helper()
	e := setupCompletionTranscript(t, "Priya Raman: Ines, Morgan and Pat, I will send the recording to all three of you.")
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		e.GrantRole(t, seat, "rep")
	}
	refs := []string{}
	for _, name := range []string{"Ines Huber", "Morgan Recipient", "Pat Buyer"} {
		contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, OwnerID: new(ids.From[ids.UserKind](e.Rep2))})
		if err != nil {
			t.Fatal(err)
		}
		e.linkTranscriptContact(t, ids.UUID(contact.Id))
		refs = append(refs, "contact:"+ids.UUID(contact.Id).String())
	}
	readFor := func(recipients int) {
		t.Helper()
		raw, err := json.Marshal(map[string][]proposedStep{"proposals": {{
			Summary: "Send the recording", Owner: "Priya Raman", PromiserRef: "seat:" + e.Rep2.String(), RecipientRefs: refs[:recipients],
			SourceLines: []int{1}, Confidence: 0.95, Meaning: openCommitmentMeaning("", ""),
		}}})
		if err != nil {
			t.Fatal(err)
		}
		e.readAt(t, cannedBrain{reply: string(raw)}, func() time.Time { return completionReadTime })
	}
	handTo := func(seat ids.UUID) {
		t.Helper()
		updateCampaignTask(t, e, activities.UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](seat))})
	}
	return e, readFor, handTo
}
