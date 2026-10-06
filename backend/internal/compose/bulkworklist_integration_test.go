// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Marking Worklist tasks and promises done in one change, over a real
// Postgres: each row through its own module's write, one audit row each, and
// one undo that opens them all again.

import (
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedWorklistTask files one open task for Rep1 through the task writer, and
// answers it as the item its Worklist row carries.
func seedWorklistTask(t *testing.T, e *integration.Env, contact ids.UUID, subject string) crmcontracts.BulkItem {
	t.Helper()
	owner := ids.From[ids.UserKind](e.Rep1)
	due := time.Now().Add(-time.Hour)
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &owner, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("seeding task %q: %v", subject, err)
	}
	return crmcontracts.BulkItem{Id: task.Id, Version: *task.Version}
}

// seedWorklistPromise records one open promise to contact through the claim
// writer, quoting a message logged for it.
func seedWorklistPromise(t *testing.T, e *integration.Env, contact ids.UUID, body string) crmcontracts.BulkItem {
	t.Helper()
	subject := "Questions about the offer"
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the message the promise was made in: %v", err)
	}
	due := time.Now().Add(time.Hour)
	claim, err := contacts.NewStore(e.DB()).RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: "commitment_ours",
		Body: body, ActivityID: ids.UUID(message.Id), Quote: body, DueAt: &due, Source: "manual",
	})
	if err != nil {
		t.Fatalf("recording the promise: %v", err)
	}
	version := e.WsCount(t, `SELECT version FROM conversation_claim WHERE id = $1`, claim.Id)
	return crmcontracts.BulkItem{Id: claim.Id, Version: int64(version)}
}

func completeItems(items ...crmcontracts.BulkItem) bulkChange {
	return bulkChange{recordType: crmcontracts.BulkRecordTypeWorklistItem, verb: crmcontracts.BulkVerbComplete, items: items}
}

func taskDoneState(t *testing.T, e *integration.Env, id openapi_types.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1`, id)
}

func TestBulkCompletingThreeTasksAndUndoingOpensAllThreeAgain(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Herr Vogt", &e.Rep1)
	items := make([]crmcontracts.BulkItem, 3)
	for i := range items {
		items[i] = seedWorklistTask(t, e, contact, fmt.Sprintf("Call back %d", i))
	}
	engine := bulkEngineFor(e)

	preview, err := engine.Preview(e.Admin(), completeItems(items...))
	if err != nil || preview.Count != 3 || len(preview.Excluded) != 0 {
		t.Fatalf("preview → %+v, %v; want all three tasks", preview, err)
	}
	if s := preview.Sample[0]; s.Before.Done == nil || *s.Before.Done || s.After.Done == nil || !*s.After.Done {
		t.Errorf("the sample row does not say the task becomes done: %+v", s)
	}
	for _, item := range items {
		if got := taskDoneState(t, e, item.Id); got != "false" {
			t.Fatalf("the preview left task %s done=%s", item.Id, got)
		}
	}

	out, err := engine.Execute(e.Admin(), completeItems(items...))
	if err != nil || out.Changed != 3 || len(out.Skipped) != 0 {
		t.Fatalf("execute → %+v, %v; want three tasks done", out, err)
	}
	for _, item := range items {
		if got := taskDoneState(t, e, item.Id); got != "true" {
			t.Errorf("task %s done=%s after the change, want true", item.Id, got)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND entity_type = 'activity'`,
		ids.UUID(out.BatchId)); n != 3 {
		t.Errorf("%d activity audit rows carry the batch id, want one per task", n)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(out.BatchId), "")
	if err != nil || undone.Changed != 3 {
		t.Fatalf("undo → %+v, %v; want all three opened again", undone, err)
	}
	for _, item := range items {
		if got := taskDoneState(t, e, item.Id); got != "false" {
			t.Errorf("task %s done=%s after the undo, want false", item.Id, got)
		}
	}
}

// One change over a task and a promise settles each through its own writer,
// and reports rather than overwrites the rows it cannot honestly change.
func TestABulkCompletionSettlesAPromiseAndReportsWhatItLeftAlone(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Frau Adler", &e.Rep1)
	task := seedWorklistTask(t, e, contact, "Send the quote")
	promise := seedWorklistPromise(t, e, contact, "Confirm the meeting")
	stale := seedWorklistTask(t, e, contact, "Edited since it was shown")
	finished := seedWorklistTask(t, e, contact, "Already done")
	edited, done := "Edited elsewhere", true
	activityID := func(item crmcontracts.BulkItem) ids.ActivityID { return ids.From[ids.ActivityKind](ids.UUID(item.Id)) }
	if _, err := e.Activities.UpdateActivity(e.Admin(), activityID(stale), activities.UpdateActivityInput{Subject: &edited}); err != nil {
		t.Fatalf("editing a task after it was shown: %v", err)
	}
	completed, err := e.Activities.UpdateActivity(e.Admin(), activityID(finished), activities.UpdateActivityInput{IsDone: &done})
	if err != nil {
		t.Fatalf("completing a task beforehand: %v", err)
	}
	finished.Version = *completed.Version
	engine := bulkEngineFor(e)

	out, err := engine.Execute(e.Admin(), completeItems(task, promise, stale, finished))
	if err != nil || out.Changed != 2 {
		t.Fatalf("execute → %+v, %v; want the task and the promise done", out, err)
	}
	assertReasons(t, "execute", skipReasons(out.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		stale.Id:    crmcontracts.BulkSkipReasonChangedSincePreview,
		finished.Id: crmcontracts.BulkSkipReasonNoChange,
	})
	if got := e.WsScalar(t, `SELECT status FROM conversation_claim WHERE id = $1`, promise.Id); got != "done" {
		t.Fatalf("the promise is %s after the change, want done", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND entity_type = 'contact'`,
		ids.UUID(out.BatchId)); n != 1 {
		t.Errorf("%d contact audit rows carry the batch id, want the one settlement", n)
	}

	if _, err := engine.Undo(e.Admin(), ids.UUID(out.BatchId), ""); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := e.WsScalar(t, `SELECT status FROM conversation_claim WHERE id = $1`, promise.Id); got != "open" {
		t.Errorf("the promise is %s after the undo, want open", got)
	}
	if got := taskDoneState(t, e, task.Id); got != "false" {
		t.Errorf("the task is done=%s after the undo, want false", got)
	}
	if got := taskDoneState(t, e, finished.Id); got != "true" {
		t.Error("the undo opened a task the change never completed")
	}
}

// settledPromise is a promise already marked done, as the item a Worklist row
// loaded before that would carry, so a change over it skips it as no_change.
func settledPromise(t *testing.T, e *integration.Env, contact ids.UUID) crmcontracts.BulkItem {
	t.Helper()
	promise := seedWorklistPromise(t, e, contact, "Send the price list")
	if err := contacts.NewStore(e.DB()).SettleConversationClaim(e.Admin(), ids.UUID(promise.Id), "done"); err != nil {
		t.Fatalf("settling the promise beforehand: %v", err)
	}
	promise.Version = int64(e.WsCount(t, `SELECT version FROM conversation_claim WHERE id = $1`, promise.Id))
	return promise
}

// A batch's stored skips name promises; a requester who can no longer read
// activities, which the Worklist's own promise read requires, is no longer told
// about them.
func TestABatchStopsNamingAPromiseOnceItsRequesterLosesActivityRead(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Herr Baum", &e.Rep1)
	task := seedWorklistTask(t, e, contact, "Call the buyer")
	promise := settledPromise(t, e, contact)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	engine := bulkEngineFor(e)

	out, err := engine.Execute(rep, completeItems(task, promise))
	if err != nil || out.Changed != 1 {
		t.Fatalf("execute → %+v, %v; want the task done and the promise skipped", out, err)
	}
	status, err := engine.Status(rep, ids.UUID(out.BatchId))
	if err != nil || len(status.Skipped) != 1 || status.Skipped[0].Id != promise.Id {
		t.Fatalf("status while the rep reads activities → %+v, %v; want the promise named", status.Skipped, err)
	}

	noActivityRead := principal.Permissions{
		RoleKeys: integration.SchedulerPerms.RoleKeys,
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
		RowScope: integration.SchedulerPerms.RowScope,
	}
	status, err = engine.Status(e.As(e.Rep1, []ids.UUID{e.Team1}, noActivityRead), ids.UUID(out.BatchId))
	if err != nil || len(status.Skipped) != 0 {
		t.Fatalf("status without activity read → %+v, %v; want the promise withheld", status.Skipped, err)
	}
}

// An agent may mark the user's tasks done, never their promises: whether a
// human kept their word is theirs to say, and the skip says so by name.
func TestAnAgentsBulkCompletionLeavesPromisesToTheUser(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Frau Keller", &e.Rep1)
	task := seedWorklistTask(t, e, contact, "Book the site visit")
	promise := seedWorklistPromise(t, e, contact, "Confirm the delivery date")
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)

	out, err := bulkEngineFor(e).Execute(agent, completeItems(task, promise))
	if err != nil || out.Changed != 1 || len(out.Skipped) != 1 {
		t.Fatalf("execute → %+v, %v; want the task done and the promise skipped", out, err)
	}
	if skip := out.Skipped[0]; skip.Id != promise.Id || skip.Reason != crmcontracts.BulkSkipReasonRefused ||
		skip.Code == nil || *skip.Code != "commitment_needs_the_user" {
		t.Errorf("the promise was skipped as %+v, want refused with commitment_needs_the_user", skip)
	}
	if got := e.WsScalar(t, `SELECT status FROM conversation_claim WHERE id = $1`, promise.Id); got != "open" {
		t.Errorf("the promise is %s, want it left open", got)
	}
}

// The Worklist offers Done, and with it the Mark done checkbox, only on a row
// the reader's write would be admitted on.
func TestTheWorklistOffersDoneOnlyWhereTheReaderMayWrite(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Herr Vogt", &e.Rep1)
	task := seedWorklistTask(t, e, contact, "Call back")
	seedWorklistPromise(t, e, contact, "Send the quote")
	readOnly := principal.Permissions{
		RoleKeys: integration.SchedulerPerms.RoleKeys,
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "activity": {Read: true},
		},
		RowScope: integration.SchedulerPerms.RowScope,
	}
	due := time.Now().Add(24 * time.Hour)
	for name, tc := range map[string]struct {
		perms    principal.Permissions
		writable bool
	}{
		"a rep who may change them": {integration.SchedulerPerms, true},
		"a rep who may only read":   {readOnly, false},
	} {
		reader := e.As(e.Rep1, []ids.UUID{e.Team1}, tc.perms)
		tasks, err := e.Activities.WritableTasks(reader, []ids.UUID{ids.UUID(task.Id)})
		if err != nil || tasks[ids.UUID(task.Id)] != tc.writable {
			t.Errorf("%s: task writable = %v (%v), want %v", name, tasks[ids.UUID(task.Id)], err, tc.writable)
		}
		claims := contacts.NewStore(e.DB())
		promises, err := claims.OpenCommitmentsDue(reader, ids.From[ids.UserKind](e.Rep1), due, 10)
		if err == nil {
			err = claims.MarkSettleable(reader, promises)
		}
		if err != nil || len(promises) != 1 || promises[0].Settleable != tc.writable {
			t.Errorf("%s: promises %+v (%v), want one with settleable = %v", name, promises, err, tc.writable)
		}
	}
}
