// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A bulk change over a Shortlist, and what a record's archive does to the
// Shortlists it was on: every membership that comes or goes is recorded, with
// who, why and the note, by the list's one membership writer.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// membershipEvents counts the list's membership events of one action and
// reason.
func membershipEvents(t *testing.T, e *integration.Env, list ids.UUID, action, reason string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM list_member_event
		WHERE list_id = $1 AND action = $2 AND reason = $3`, list, action, reason)
}

func TestArchivingAMemberRecordsItsRemovalAndTheUndoItsReturn(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedSurroundedContacts(t, e, 3)
	list := seeded[0].list
	engine := bulkEngineFor(e)

	archived, err := engine.Execute(e.Admin(), archiveContacts(itemsOf(seeded)))
	if err != nil || archived.Changed != 3 {
		t.Fatalf("archiving three → %+v, %v", archived, err)
	}
	if members := e.WsCount(t, `SELECT count(*) FROM list_member WHERE list_id = $1`, list); members != 0 {
		t.Fatalf("%d memberships outlived their archived records", members)
	}
	if removed := membershipEvents(t, e, list, "removed", "record_archived"); removed != 3 {
		t.Fatalf("the archive recorded %d removals, want one per membership it took down", removed)
	}

	if _, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), ""); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if back := membershipEvents(t, e, list, "added", "record_restored"); back != 3 {
		t.Fatalf("the undo recorded %d returns, want one per membership it put back", back)
	}
}

// addToShortlist is a bulk change adding contacts to a Shortlist with a note.
func addToShortlist(items []crmcontracts.BulkItem, list ids.ListID, note string) bulkChange {
	id := list.UUID
	return bulkChange{
		recordType: crmcontracts.BulkRecordTypeContact, verb: crmcontracts.BulkVerbAddToList,
		items: items, listID: &id, note: &note,
	}
}

func TestABulkChangeAddsASelectionToAShortlistAndItsUndoTakesItOff(t *testing.T) {
	e := integration.Setup(t)
	lists := collections.NewStore(e.DB())
	list, err := lists.CreateList(e.Admin(), collections.CreateListInput{Name: "Launch references", EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	items := seedBulkContacts(t, e, e.Rep1, 3)
	engine := bulkEngineFor(e).withLists(lists)

	preview, err := engine.Preview(e.Admin(), addToShortlist(items, list.ID, "approved references"))
	if err != nil || preview.Count != 3 {
		t.Fatalf("preview → %+v, %v", preview, err)
	}
	sample := preview.Sample[0]
	if sample.Before.Listed == nil || *sample.Before.Listed || sample.After.Listed == nil || !*sample.After.Listed {
		t.Fatalf("the sample row does not say the record joins the list: %+v", sample)
	}
	if members := e.WsCount(t, `SELECT count(*) FROM list_member WHERE list_id = $1`, list.ID); members != 0 {
		t.Fatalf("a preview wrote %d memberships", members)
	}

	added, err := engine.Execute(e.Admin(), addToShortlist(items, list.ID, "approved references"))
	if err != nil || added.Changed != 3 {
		t.Fatalf("execute → %+v, %v", added, err)
	}
	noted := e.WsCount(t, `SELECT count(*) FROM list_member_event
		WHERE list_id = $1 AND action = 'added' AND reason = 'bulk' AND note = 'approved references'`, list.ID)
	if noted != 3 {
		t.Fatalf("%d of three additions were recorded as a bulk change with its note", noted)
	}
	again, err := engine.Execute(e.Admin(), addToShortlist(items, list.ID, "again"))
	if err != nil || again.Changed != 0 || len(again.Skipped) != 3 || again.Skipped[0].Reason != crmcontracts.BulkSkipReasonNoChange {
		t.Fatalf("adding members already on the list → %+v, %v, want three no_change skips", again, err)
	}

	if _, err := engine.Undo(e.Admin(), ids.UUID(added.BatchId), ""); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if members := e.WsCount(t, `SELECT count(*) FROM list_member WHERE list_id = $1`, list.ID); members != 0 {
		t.Fatalf("the undo left %d of the added members on the list", members)
	}
	if removed := membershipEvents(t, e, list.ID.UUID, "removed", "bulk"); removed != 3 {
		t.Fatalf("the undo recorded %d removals, want three", removed)
	}
}

func TestABulkListVerbIsRefusedBeforeAnyRowIsTried(t *testing.T) {
	e := integration.Setup(t)
	lists := collections.NewStore(e.DB())
	list, err := lists.CreateList(e.As(e.Rep1, []ids.UUID{e.Team1}, bulkListPerms()), collections.CreateListInput{
		Name: "Rep1's targets", EntityType: "contact",
	})
	if err != nil {
		t.Fatal(err)
	}
	items := seedBulkContacts(t, e, e.Rep1, 2)
	change := addToShortlist(items, list.ID, "")

	if _, err := bulkEngineFor(e).Preview(e.Admin(), change); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a list verb with lists switched off answered %v, want not found", err)
	}
	teammate := e.As(e.Rep2, []ids.UUID{e.Team1}, bulkListPerms())
	if _, err := bulkEngineFor(e).withLists(lists).Preview(teammate, change); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a teammate who is not the steward answered %v, want permission denied", err)
	}
}

// bulkListPerms is a team-scoped rep who reads contacts and works lists.
func bulkListPerms() principal.Permissions {
	p := integration.RepPerms
	p.Objects = map[string]principal.ObjectGrant{
		"contact": {Create: true, Read: true, Update: true},
		"list":    {Create: true, Read: true, Update: true, Delete: true},
	}
	return p
}
