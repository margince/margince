// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestHistoricalTaskSurvivesMissingParticipantsAndReassignment(t *testing.T) {
	e := setupTranscript(t)
	meeting, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", SourceSystem: new("transcript"), Body: new(transcriptBody),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	quote := strings.Split(transcriptBody, "\n")[2]
	locator := commitmentLocator(e.activity.UUID, claimKindOurs, "seat:"+e.Rep2.String(), "3:"+quote)
	ctx := withTranscriptReader(e.ctx, "human:"+e.Rep1.String(), ids.NewV7())
	var task ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		task, err = writeCommitmentTask(ctx, tx, e.Activities, e.Contacts, commitmentTask{
			Extractor: transcriptProposalActor, Locator: locator, Summary: "Send revised pricing", Body: quote,
			SourceActivityID: e.activity.UUID, Assignee: e.Rep2, Origin: activities.OriginAgent,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](task), activities.UpdateActivityInput{
		AssigneeID: new(ids.From[ids.UserKind](e.Rep1)),
	}); err != nil {
		t.Fatal(err)
	}
	integration.HoldUnderStatutoryFloor(t, e.Env, task)
	again := e.read(t, cannedBrain{reply: ownedReply(t, "the team", 0.95)})
	if len(again.ProposalIDs) != 0 || e.taskCount(t) != 1 {
		t.Fatal("missing participants or changed attribution repeated the historical task")
	}
}

func TestCustomerPromiseDoesNotBecomeOurTaskWhenAttributionIsLost(t *testing.T) {
	for _, state := range []string{"open", "dismissed", "archived contact"} {
		t.Run(state, func(t *testing.T) {
			e := setupTranscript(t)
			contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Frédéric de Gombert"})
			if err != nil {
				t.Fatal(err)
			}
			e.linkTranscriptContact(t, ids.UUID(contact.Id))
			e.read(t, cannedBrain{reply: attributedReply(t, "Frédéric de Gombert", "contact:"+contact.Id.String(), 0.95)})
			if state != "open" {
				claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim`))
				if err != nil {
					t.Fatal(err)
				}
				if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, "dismissed"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "archived contact" {
				if _, err := e.Contacts.ArchiveContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(contact.Id)), nil); err != nil {
					t.Fatal(err)
				}
			}
			again := e.read(t, cannedBrain{reply: ownedReply(t, "someone", 0.95)})
			if len(again.ProposalIDs) != 0 || e.taskCount(t) != 0 {
				t.Fatal("an unresolved reread converted the customer's promise into our work")
			}
			if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim`); got != 1 {
				t.Fatalf("rereading left %d claims for one promise", got)
			}
		})
	}
}

// An unattributed promise asks nobody, and once it is found to be the
// customer's it is filed on them as theirs.
func TestAnUnattributedPromiseLaterFoundToBeTheCustomersIsFiledOnThem(t *testing.T) {
	e := setupTranscript(t)
	first := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
	if len(first.ProposalIDs) != 0 || e.taskCount(t) != 0 {
		t.Fatal("an unattributed promise became our work")
	}
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Priya Raman"})
	if err != nil {
		t.Fatal(err)
	}
	e.linkTranscriptContact(t, ids.UUID(contact.Id))
	again := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "contact:"+contact.Id.String(), 0.95)})
	if len(again.ProposalIDs) != 0 || e.taskCount(t) != 0 {
		t.Fatal("customer attribution recreated our work")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE kind='commitment_theirs' AND contact_id=$1`, ids.UUID(contact.Id)); got != 1 {
		t.Fatalf("the customer's promise is filed %d times, want once", got)
	}
}
