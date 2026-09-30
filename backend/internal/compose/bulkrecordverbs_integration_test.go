// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The tag verbs, create_task and leads, through the engine over a real
// Postgres: what each row is refused for, what one batch writes, and what its
// undo puts back.

import (
	"errors"
	"fmt"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedBulkTag coins a tag through the tag writer.
func seedBulkTag(t *testing.T, e *integration.Env) ids.TagID {
	t.Helper()
	tag, err := collections.NewStore(e.DB()).CreateTag(e.Admin(), "Bulk tag "+ids.NewV7().String()[:8], nil, nil)
	if err != nil {
		t.Fatalf("seeding a tag: %v", err)
	}
	return tag.ID
}

func tagChange(verb crmcontracts.BulkVerb, tag ids.TagID, items []crmcontracts.BulkItem) bulkChange {
	id := tag.UUID
	return bulkChange{recordType: crmcontracts.BulkRecordTypeContact, verb: verb, items: items, tagID: &id}
}

func taggedCount(t *testing.T, e *integration.Env, tag ids.TagID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id = $1`, tag)
}

// bulkTagPerms is a team-scoped rep who may change contacts and read the tag
// vocabulary — what tagging a record takes.
func bulkTagPerms() principal.Permissions {
	p := integration.RepPerms
	p.Objects = map[string]principal.ObjectGrant{
		"contact":  {Create: true, Read: true, Update: true},
		"tag":      {Read: true},
		"activity": {Create: true, Read: true, Update: true, Delete: true},
	}
	return p
}

func TestABulkTagPutsATagOnTheSelectionAndItsUndoTakesOffOnlyWhatItAdded(t *testing.T) {
	e := integration.Setup(t)
	tag := seedBulkTag(t, e)
	items := seedBulkContacts(t, e, e.Rep1, 3)
	already := items[0]
	if _, err := collections.NewStore(e.DB()).ApplyTag(e.Admin(), tag, "contact", ids.UUID(already.Id)); err != nil {
		t.Fatalf("tagging one contact beforehand: %v", err)
	}
	engine := bulkEngineFor(e)

	preview, err := engine.Preview(e.Admin(), tagChange(crmcontracts.BulkVerbAddTag, tag, items))
	if err != nil || preview.Count != 2 {
		t.Fatalf("preview → %+v, %v; want the two untagged contacts", preview, err)
	}
	assertReasons(t, "preview", skipReasons(preview.Excluded),
		map[openapi_types.UUID]crmcontracts.BulkSkipReason{already.Id: crmcontracts.BulkSkipReasonNoChange})
	if s := preview.Sample[0]; s.Before.Tagged == nil || *s.Before.Tagged || s.After.Tagged == nil || !*s.After.Tagged {
		t.Errorf("the sample row does not say the record gains the tag: %+v", s)
	}
	if n := taggedCount(t, e, tag); n != 1 {
		t.Fatalf("the preview left %d taggings, want only the one seeded", n)
	}

	out, err := engine.Execute(e.Admin(), tagChange(crmcontracts.BulkVerbAddTag, tag, items))
	if err != nil || out.Changed != 2 {
		t.Fatalf("execute → %+v, %v", out, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND entity_type = 'tag'`,
		ids.UUID(out.BatchId)); n != 2 {
		t.Errorf("%d tag audit rows carry the batch id, want one per tagged record", n)
	}

	if _, err := engine.Undo(e.Admin(), ids.UUID(out.BatchId), ""); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if n := taggedCount(t, e, tag); n != 1 {
		t.Fatalf("after the undo %d contacts carry the tag, want only the one tagged before the change", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_id = $2`, tag, already.Id); n != 1 {
		t.Error("the undo took the tag off a contact the change never tagged")
	}
}

// A tag somebody took off and put back after the batch is their assignment,
// not the batch's: the undo leaves it and says why.
func TestUndoingABulkTagLeavesATagSomebodyPutBackSince(t *testing.T) {
	e := integration.Setup(t)
	tag := seedBulkTag(t, e)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	engine := bulkEngineFor(e)
	out, err := engine.Execute(e.Admin(), tagChange(crmcontracts.BulkVerbAddTag, tag, items))
	if err != nil || out.Changed != 2 {
		t.Fatalf("execute → %+v, %v", out, err)
	}
	tags := collections.NewStore(e.DB())
	reapplied := ids.UUID(items[0].Id)
	if err := tags.RemoveTag(e.Admin(), tag, "contact", reapplied); err != nil {
		t.Fatal(err)
	}
	if _, err := tags.ApplyTag(e.Admin(), tag, "contact", reapplied); err != nil {
		t.Fatal(err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(out.BatchId), "")
	if err != nil || undone.Changed != 1 {
		t.Fatalf("undo → %+v, %v; want only the untouched assignment removed", undone, err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped),
		map[openapi_types.UUID]crmcontracts.BulkSkipReason{items[0].Id: crmcontracts.BulkSkipReasonChangedSinceBatch})
	if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_id = $2`, tag, reapplied); n != 1 {
		t.Error("the undo removed a tag somebody put back after the batch")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_id = $2`, tag, items[1].Id); n != 0 {
		t.Error("the undo left the batch's own assignment in place")
	}
}

func TestABulkRemoveTagAndItsUndoPutsItBack(t *testing.T) {
	e := integration.Setup(t)
	tag := seedBulkTag(t, e)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	tags := collections.NewStore(e.DB())
	if _, err := tags.ApplyTag(e.Admin(), tag, "contact", ids.UUID(items[0].Id)); err != nil {
		t.Fatal(err)
	}
	engine := bulkEngineFor(e)
	out, err := engine.Execute(e.Admin(), tagChange(crmcontracts.BulkVerbRemoveTag, tag, items))
	if err != nil || out.Changed != 1 || len(out.Skipped) != 1 || out.Skipped[0].Reason != crmcontracts.BulkSkipReasonNoChange {
		t.Fatalf("removing → %+v, %v; want one removed and the untagged one a no_change", out, err)
	}
	if n := taggedCount(t, e, tag); n != 0 {
		t.Fatalf("%d taggings survived the removal", n)
	}
	if _, err := engine.Undo(e.Admin(), ids.UUID(out.BatchId), ""); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_id = $2`, tag, items[0].Id); n != 1 {
		t.Error("the undo did not put the tag back on the contact it was taken off")
	}
	if n := taggedCount(t, e, tag); n != 1 {
		t.Errorf("after the undo %d contacts carry the tag, want only the one it was taken off", n)
	}
}

// The per-row write check: a rep may tag their own team's contact and not a
// contact they may only read, and a record nobody can find is not found.
func TestABulkTagLeavesARecordTheCallerMayOnlyReadAlone(t *testing.T) {
	e := integration.Setup(t)
	tag := seedBulkTag(t, e)
	own := seedBulkContacts(t, e, e.Rep1, 1)
	foreign := seedBulkContacts(t, e, e.Rep3, 1)
	missing := crmcontracts.BulkItem{Id: openapi_types.UUID(ids.NewV7()), Version: 1}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, bulkTagPerms())

	out, err := bulkEngineFor(e).Execute(rep1, tagChange(crmcontracts.BulkVerbAddTag, tag, []crmcontracts.BulkItem{own[0], foreign[0], missing}))
	if err != nil || out.Changed != 1 {
		t.Fatalf("execute → %+v, %v; want only the rep's own contact tagged", out, err)
	}
	assertReasons(t, "execution", skipReasons(out.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		foreign[0].Id: crmcontracts.BulkSkipReasonNotWritable,
		missing.Id:    crmcontracts.BulkSkipReasonNotFound,
	})
	if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE tag_id = $1 AND entity_id = $2`, tag, foreign[0].Id); n != 0 {
		t.Error("the contact the rep may only read was tagged")
	}
}

func TestATagVerbNamingNoLiveTagIsRefusedBeforeAnyRowIsTried(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 1)
	retired := seedBulkTag(t, e)
	if _, err := collections.NewStore(e.DB()).ArchiveTag(e.Admin(), retired); err != nil {
		t.Fatal(err)
	}
	engine := bulkEngineFor(e)
	for name, tag := range map[string]ids.TagID{"unknown": ids.From[ids.TagKind](ids.NewV7()), "retired": retired} {
		if _, err := engine.Preview(e.Admin(), tagChange(crmcontracts.BulkVerbAddTag, tag, items)); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("adding a %s tag answered %v, want not found", name, err)
		}
	}
	if _, err := engine.Preview(e.Admin(), tagChange(crmcontracts.BulkVerbRemoveTag, retired, items)); err != nil {
		t.Errorf("taking a retired tag off answered %v; a retired tag stays removable", err)
	}
}

func taskChange(items []crmcontracts.BulkItem, assignee *ids.UUID) bulkChange {
	task := &crmcontracts.BulkTask{Subject: "Call back about the renewal", AssigneeId: wireOwner(assignee)}
	return bulkChange{recordType: crmcontracts.BulkRecordTypeContact, verb: crmcontracts.BulkVerbCreateTask, items: items, task: task}
}

// tasksFiled counts the live tasks the batch's audit rows name, linked to the
// records they were filed under.
func tasksFiled(t *testing.T, e *integration.Env, batch openapi_types.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.contact_id IS NOT NULL
		WHERE a.kind = 'task' AND a.archived_at IS NULL
		  AND a.id IN (SELECT entity_id FROM audit_log WHERE batch_id = $1 AND entity_type = 'activity')`, batch)
}

func TestABulkCreateTaskFilesOneTaskPerRecordAndItsUndoArchivesThem(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 3)
	engine := bulkEngineFor(e)
	assignee := e.Rep2

	preview, err := engine.Preview(e.Admin(), taskChange(items, &assignee))
	if err != nil || preview.Count != 3 {
		t.Fatalf("preview → %+v, %v", preview, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND subject = 'Call back about the renewal'`); n != 0 {
		t.Fatalf("the preview filed %d tasks", n)
	}
	out, err := engine.Execute(e.Admin(), taskChange(items, &assignee))
	if err != nil || out.Changed != 3 {
		t.Fatalf("execute → %+v, %v", out, err)
	}
	if n := tasksFiled(t, e, out.BatchId); n != 3 {
		t.Fatalf("%d live tasks trace to the batch, want one per contact", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND assignee_id = $1
		AND subject = 'Call back about the renewal'`, assignee); n != 3 {
		t.Errorf("%d tasks are owed by the named colleague, want three", n)
	}

	// A task somebody finished since is their work now: the undo leaves it.
	done := e.WsScalar(t, `SELECT a.id::text FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.contact_id = $1 AND a.kind = 'task'`, items[0].Id)
	doneID, err := ids.ParseAs[ids.ActivityKind](done)
	if err != nil {
		t.Fatal(err)
	}
	finished := true
	if _, err := e.Activities.UpdateActivity(e.Admin(), doneID, activities.UpdateActivityInput{IsDone: &finished}); err != nil {
		t.Fatalf("finishing one task: %v", err)
	}
	undone, err := engine.Undo(e.Admin(), ids.UUID(out.BatchId), "")
	if err != nil || undone.Changed != 2 {
		t.Fatalf("undo → %+v, %v; want the two untouched tasks archived", undone, err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped),
		map[openapi_types.UUID]crmcontracts.BulkSkipReason{items[0].Id: crmcontracts.BulkSkipReasonChangedSinceBatch})
	if n := tasksFiled(t, e, out.BatchId); n != 1 {
		t.Errorf("%d of the batch's tasks are still live after the undo, want only the finished one", n)
	}
}

func TestABulkCreateTaskRefusesAnAssigneeWhoCannotHoldWork(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 1)
	nobody := ids.NewV7()
	_, err := bulkEngineFor(e).Execute(e.Admin(), taskChange(items, &nobody))
	var refused *auth.AssigneeNotAllowedError
	if !errors.As(err, &refused) {
		t.Fatalf("an assignee who is no colleague answered %v, want a refusal before any row", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND subject = 'Call back about the renewal'`); n != 0 {
		t.Errorf("%d tasks were filed for an assignee nobody may hand work to", n)
	}
}

// The per-row read check: a task is filed only under a record the caller can
// see. Contacts are readable by every seat, so the hidden one is capture-private.
func TestABulkCreateTaskSkipsARecordTheCallerCannotSee(t *testing.T) {
	e := integration.Setup(t)
	own := seedBulkContacts(t, e, e.Rep1, 1)
	foreign := seedBulkContacts(t, e, e.Rep3, 1)
	e.MakeCapturePrivate(t, "contact", ids.UUID(foreign[0].Id), e.Rep3)
	foreign[0].Version = int64(e.WsCount(t, `SELECT version FROM contact WHERE id = $1`, foreign[0].Id))
	missing := crmcontracts.BulkItem{Id: openapi_types.UUID(ids.NewV7()), Version: 1}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, bulkTagPerms())
	out, err := bulkEngineFor(e).Execute(rep1, taskChange([]crmcontracts.BulkItem{own[0], foreign[0], missing}, nil))
	if err != nil || out.Changed != 1 {
		t.Fatalf("execute → %+v, %v", out, err)
	}
	assertReasons(t, "execution", skipReasons(out.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		foreign[0].Id: crmcontracts.BulkSkipReasonNotFound,
		missing.Id:    crmcontracts.BulkSkipReasonNotFound,
	})
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND assignee_id = $1
		AND subject = 'Call back about the renewal'`, e.Rep1); n != 1 {
		t.Errorf("%d tasks were filed for the rep, want one, owed by the rep who asked", n)
	}
}

// seedBulkLeads creates n leads owned by owner through the lead writer.
func seedBulkLeads(t *testing.T, e *integration.Env, owner ids.UUID, n int) []crmcontracts.BulkItem {
	t.Helper()
	items := make([]crmcontracts.BulkItem, 0, n)
	for i := range n {
		name := fmt.Sprintf("Bulk Lead %d %s", i, ids.NewV7().String()[:8])
		ownerID := ids.From[ids.UserKind](owner)
		created, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, OwnerID: &ownerID, Source: "manual"})
		if err != nil {
			t.Fatalf("seeding lead %d: %v", i, err)
		}
		items = append(items, crmcontracts.BulkItem{Id: created.Id, Version: *created.Version})
	}
	return items
}

// A lead known only by its address is still a row of the batch, labelled by
// that address, rather than a NULL that aborts every other row.
func TestABulkChangeOverAnUnnamedLeadLabelsItByItsAddress(t *testing.T) {
	e := integration.Setup(t)
	email := "unnamed-" + ids.NewV7().String()[:8] + "@example.com"
	ownerID := ids.From[ids.UserKind](e.Rep1)
	unnamed, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Email: &email, OwnerID: &ownerID, Source: "manual"})
	if err != nil {
		t.Fatalf("seeding an unnamed lead: %v", err)
	}
	items := append(seedBulkLeads(t, e, e.Rep1, 1), crmcontracts.BulkItem{Id: unnamed.Id, Version: *unnamed.Version})
	owner := e.Rep2
	preview, err := bulkEngineFor(e).Preview(e.Admin(), bulkChange{
		recordType: crmcontracts.BulkRecordTypeLead, verb: crmcontracts.BulkVerbReassignOwner, items: items, ownerID: &owner,
	})
	if err != nil || preview.Count != 2 {
		t.Fatalf("preview over an unnamed lead → %+v, %v; want both leads", preview, err)
	}
	labels := map[openapi_types.UUID]string{}
	for _, row := range preview.Sample {
		labels[row.Id] = row.Label
	}
	if labels[unnamed.Id] != email {
		t.Errorf("the unnamed lead is labelled %q, want its address", labels[unnamed.Id])
	}
}

func TestLeadsTakeEveryBulkVerbButArchive(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	leads := seedBulkLeads(t, e, e.Rep1, 2)
	owner := e.Rep2

	moved, err := engine.Execute(e.Admin(), bulkChange{
		recordType: crmcontracts.BulkRecordTypeLead, verb: crmcontracts.BulkVerbReassignOwner, items: leads, ownerID: &owner,
	})
	if err != nil || moved.Changed != 2 {
		t.Fatalf("reassigning two leads → %+v, %v", moved, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE owner_id = $1`, owner); n != 2 {
		t.Errorf("%d leads moved to the new owner, want two", n)
	}
	if _, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), ""); err != nil {
		t.Fatalf("undoing the lead reassignment: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE owner_id = $1`, e.Rep1); n != 2 {
		t.Errorf("%d leads went back to their owner, want two", n)
	}

	fresh := seedBulkLeads(t, e, e.Rep1, 1)
	tag := seedBulkTag(t, e)
	tagged := tagChange(crmcontracts.BulkVerbAddTag, tag, fresh)
	tagged.recordType = crmcontracts.BulkRecordTypeLead
	if out, err := engine.Execute(e.Admin(), tagged); err != nil || out.Changed != 1 {
		t.Errorf("tagging a lead → %+v, %v", out, err)
	}
	task := taskChange(fresh, nil)
	task.recordType = crmcontracts.BulkRecordTypeLead
	if out, err := engine.Execute(e.Admin(), task); err != nil || out.Changed != 1 {
		t.Errorf("filing a task under a lead → %+v, %v", out, err)
	}

	_, err = engine.Preview(e.Admin(), bulkChange{recordType: crmcontracts.BulkRecordTypeLead, verb: crmcontracts.BulkVerbArchive, items: fresh})
	var refused *httperr.DetailedError
	if !errors.As(err, &refused) || refused.Fields[0].Code != "verb_not_for_record_type" {
		t.Errorf("archiving a lead answered %v, want verb_not_for_record_type", err)
	}
}
