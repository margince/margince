// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestHomeAndWorklistUndoTheCapturedMailCompletion(t *testing.T) {
	for _, surface := range []string{"Home", "Worklist", "Sent attestation", "Durable send"} {
		t.Run(surface, func(t *testing.T) {
			e := integration.Setup(t)
			reader := e.As(e.Rep1, nil, integration.AdminPerms)
			contact, err := e.Contacts.CreateContact(reader, contacts.CreateContactInput{FullName: "Task recipient", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "task@receipts.test", EmailType: "work", IsPrimary: true}}})
			if err != nil {
				t.Fatal(err)
			}
			task, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "task", Subject: new("Follow up"), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}}})
			if err != nil {
				t.Fatal(err)
			}
			sent := task.CreatedAt.Add(time.Hour)
			email, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "email", Subject: new("The answer"), Direction: new("outbound"), OccurredAt: &sent, EmailFrom: "rep@receipts.test", EmailTo: []string{"task@receipts.test"}, CounterpartyOutboundAttested: true, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			event := workflow.Event{ID: ids.NewV7(), Type: "activity.captured", WorkspaceID: e.WS, Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.UUID(email.Id)}, Payload: []byte(`{"kind":"email"}`)}
			handler := activities.FollowUpWorkflows(e.Activities)[0]
			if surface == "Sent attestation" {
				event.Type = "activity.updated"
				event.Payload = []byte(`{"changed_fields":{"outbound_attested":true}}`)
				handler = activities.FollowUpWorkflows(e.Activities)[3]
			}
			system := principal.SystemActing(reader, "system:completion-test")
			matched, err := handler.Match(system, event)
			if err != nil || !matched {
				t.Fatalf("capture match=%v,%v", matched, err)
			}
			plan, err := handler.Plan(system, event)
			if err != nil {
				t.Fatal(err)
			}
			if surface == "Durable send" {
				delivery := ids.NewV7()
				e.WsExec(t, `INSERT INTO comms_outbound(id,activity_id,user_id,provider,message_id,recipients,subject,body,status,consent_purpose,sender_kind,party_admission_pending) VALUES($1,$2,$3,'gmail','sent@receipts.test','["task@receipts.test"]','Answer','Answer','sent','business_correspondence','user',true)`, delivery, email.Id, e.Rep1)
				for range 2 {
					if err := sentPartySettlement(e.Pool)(system, delivery); err != nil {
						t.Fatal(err)
					}
				}
				if n := e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE id=$1 AND party_admission_pending`, delivery); n != 0 {
					t.Fatal("send derivation did not retire")
				}
			} else if _, err := handler.Apply(system, event, plan, nil); err != nil {
				t.Fatal(err)
			}
			var audit ids.UUID
			var version int64
			now := sent.Add(time.Hour)
			if surface == "Home" {
				audit, version = homeTaskCompletionUndo(reader, t, e, task.CreatedAt.Add(-time.Hour))
			} else {
				audit, version = worklistTaskCompletionUndo(reader, t, e, now)
			}
			if audit.IsZero() {
				t.Fatal("the completion receipt was missing")
			}
			if _, err := restoreSeamFor(e).Restore(reader, entityTypeActivity, ids.UUID(task.Id), audit, version); err != nil {
				t.Fatalf("%s Undo: %v", surface, err)
			}
			if _, err := handler.Apply(system, event, plan, nil); err != nil {
				t.Fatal(err)
			}
			restored, err := e.Activities.GetActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)), storekit.LiveOnly)
			if err != nil || restored.IsDone == nil || *restored.IsDone {
				t.Fatalf("replay reclosed the undone task: %+v,%v", restored, err)
			}
		})
	}
}

func homeTaskCompletionUndo(reader context.Context, t *testing.T, e *integration.Env, since time.Time) (ids.UUID, int64) {
	t.Helper()
	source := magicDecidedReceipts{receipts: attentionReceipts{tasks: e.Activities, svc: NewApprovalsService(e.DB()), deals: deals.NewStore(e.DB(), DealsInstallation())}}
	receipts, _, err := source.DecidedSince(reader, since, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range receipts {
		if receipt.Kind != "task_completed" {
			continue
		}
		if receipt.Undo == nil || !receipt.Undo.Undoable || receipt.Undo.AuditId == nil || receipt.Undo.Version == nil {
			t.Fatalf("Home has no usable Undo: %+v", receipt.Undo)
		}
		return ids.UUID(*receipt.Undo.AuditId), *receipt.Undo.Version
	}
	t.Fatal("Home completion receipt was missing")
	return ids.Nil, 0
}

func worklistTaskCompletionUndo(reader context.Context, t *testing.T, e *integration.Env, now time.Time) (ids.UUID, int64) {
	t.Helper()
	page, err := newAttentionService(e.Pool, NewApprovalsService(e.DB()), func() time.Time { return now }).HandledForYou(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range page.Receipts {
		if receipt.Kind != "task_completed" {
			continue
		}
		if receipt.Undo == nil || receipt.Undo.Reversed {
			t.Fatalf("Worklist has no usable Undo: %+v", receipt)
		}
		return ids.UUID(receipt.Undo.AuditLogId), receipt.Undo.Version
	}
	t.Fatal("Worklist completion receipt was missing")
	return ids.Nil, 0
}
