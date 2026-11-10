// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAContactTaskClosesOnLaterMailAndUndoSurvivesReplay(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	contact := e.seedContact(t)
	e.exec(t, `INSERT INTO contact_email (contact_id,email,email_type,source,captured_by) VALUES ($1,'guest@resolve.test','work','manual','human:test')`, contact)
	ctx := principal.WithActor(e.systemCtx(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep, SeatType: principal.SeatFull, Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}, "contact": {Read: true, Update: true}}}})
	subject := "Follow up"
	task, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: &subject, Source: "manual", AssigneeID: new(ids.From[ids.UserKind](e.rep)), Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	outcome, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Body: new("The call happened"), Source: "manual", SourceActivityID: new(ids.UUID(task.Id))})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.SourceActivityId == nil || *outcome.SourceActivityId != task.Id || outcome.Links == nil || len(*outcome.Links) != 1 {
		t.Fatalf("outcome lost its task or contact context: %+v", outcome)
	}
	direction := "outbound"
	sent := task.CreatedAt.Add(time.Hour)
	for _, test := range []struct {
		name, address string
		at            time.Time
	}{{"earlier", "guest@resolve.test", task.CreatedAt.Add(-time.Hour)}, {"another contact", "someoneelse@resolve.test", sent}} {
		mail, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "email", Subject: new(test.name), Direction: &direction, OccurredAt: &test.at, EmailFrom: "rep@resolve.test", EmailTo: []string{test.address}, CounterpartyOutboundAttested: true, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := store.completeContactTasksAnsweredBy(e.systemCtx(), ids.From[ids.ActivityKind](ids.UUID(mail.Id))); err != nil || n != 0 {
			t.Fatalf("%s completed %d tasks: %v", test.name, n, err)
		}
	}
	mail, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "email", Subject: new("Reply"), Body: new("Here is the answer"), Direction: &direction, OccurredAt: &sent, EmailFrom: "rep@resolve.test", EmailTo: []string{"guest@resolve.test"}, CounterpartyOutboundAttested: true, Source: "manual", Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := store.completeContactTasksAnsweredBy(e.systemCtx(), ids.From[ids.ActivityKind](ids.UUID(mail.Id))); err != nil || n != 1 {
		t.Fatalf("completed %d: %v", n, err)
	}
	if !e.isDone(t, ids.UUID(task.Id)) {
		t.Fatal("later email left the task open")
	}
	receipts, err := store.RecentTaskCompletions(ctx, task.CreatedAt.Add(-time.Hour), 10)
	if err != nil || len(receipts) != 1 || receipts[0].ID != ids.UUID(task.Id) || !receipts[0].WrittenAt.Equal(sent) {
		t.Fatalf("completion receipt=%+v, %v", receipts, err)
	}
	if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(task.Id)), UpdateActivityInput{IsDone: new(false)}); err != nil {
		t.Fatal(err)
	}
	if n, err := store.completeContactTasksAnsweredBy(e.systemCtx(), ids.From[ids.ActivityKind](ids.UUID(mail.Id))); err != nil || n != 0 {
		t.Fatalf("replayed email completed %d tasks: %v", n, err)
	}
	if e.isDone(t, ids.UUID(task.Id)) {
		t.Fatal("the same email undid the user's Undo")
	}
}
