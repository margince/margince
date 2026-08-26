// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Putting one audited change back, end to end over HTTP.
//
// The refusals are what this feature IS, so each one is proved here against
// real rows rather than against a double: a test that supplied its own version
// of the audit trail would prove nothing about the trail the product writes.

import (
	"fmt"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
)

type historyEntry struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	Undoable struct {
		Undoable bool    `json:"undoable"`
		Reason   *string `json:"reason"`
		Detail   *string `json:"detail"`
	} `json:"undoable"`
	Before   map[string]any `json:"before"`
	After    map[string]any `json:"after"`
	Evidence map[string]any `json:"evidence"`
}

type historyPage struct {
	Data []historyEntry `json:"data"`
}

type personRecord struct {
	ID       string  `json:"id"`
	Version  int64   `json:"version"`
	FullName string  `json:"full_name"`
	Title    *string `json:"title"`
}

// readHistory returns the record's history, newest LAST (the projection is
// chronological), with each entry's undoability as the surface would show it.
func readHistory(t *testing.T, e *apptest.AppEnv, entityType, id string) historyPage {
	t.Helper()
	var page historyPage
	if status := e.Call(t, "GET", "/v1/records/"+entityType+"/"+id+"/history", nil, nil, &page); status != 200 {
		t.Fatalf("read %s history → %d", entityType, status)
	}
	return page
}

func readPerson(t *testing.T, e *apptest.AppEnv, id string) personRecord {
	t.Helper()
	var person personRecord
	if status := e.Call(t, "GET", "/v1/people/"+id, nil, nil, &person); status != 200 {
		t.Fatalf("read person → %d", status)
	}
	return person
}

// theUpdateEntry is the record's newest `update` line — the one a person would
// press Undo on.
func theUpdateEntry(t *testing.T, page historyPage) historyEntry {
	t.Helper()
	for i := len(page.Data) - 1; i >= 0; i-- {
		if page.Data[i].Action == "update" {
			return page.Data[i]
		}
	}
	t.Fatalf("no update entry in a history of %d lines", len(page.Data))
	return historyEntry{}
}

func restore(t *testing.T, e *apptest.AppEnv, entityType, id, auditID string, version int64) (int, historyEntry) {
	t.Helper()
	var entry historyEntry
	status := e.Call(t, "POST",
		fmt.Sprintf("/v1/records/%s/%s/history/%s/restore", entityType, id, auditID),
		nil, map[string]string{"If-Match": fmt.Sprint(version)}, &entry)
	return status, entry
}

// The round trip: a field changed, put back, and the record equals what it held
// before — with the reversal recorded as a `restore` naming the row it reversed.
func TestEndToEnd_anAuditedChangeGoesBack(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		apptest.AnyMap{"full_name": "Greta Original", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		apptest.AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch person → %d", status)
	}

	page := readHistory(t, e, "person", created.ID)
	entry := theUpdateEntry(t, page)
	if !entry.Undoable.Undoable {
		t.Fatalf("a fresh, unsuperseded update reads as not undoable: %v", reasonOf(entry))
	}

	current := readPerson(t, e, created.ID)
	status, restoreEntry := restore(t, e, "person", created.ID, entry.ID, current.Version)
	if status != 200 {
		t.Fatalf("restore → %d, want 200", status)
	}
	if restoreEntry.Action != "restore" {
		t.Errorf("the reversal was recorded as %q, want %q", restoreEntry.Action, "restore")
	}

	back := readPerson(t, e, created.ID)
	if back.Title == nil || *back.Title != "CTO" {
		t.Errorf("title after the restore = %v, want the value it held before the change", back.Title)
	}
	// Every other field the update did not touch is untouched too — a restore
	// that quietly rewrote a field outside the entry's image would be reversing
	// more than the person asked to reverse.
	if back.FullName != "Greta Original" {
		t.Errorf("full_name = %q; the restore wrote a field outside the entry's image", back.FullName)
	}
}

// A restore of the same entry twice is refused the second time, by name. A
// second press must not silently write the same values again and mint another
// reversal nobody asked for.
func TestEndToEnd_anEntryAlreadyPutBackRefusesBySayingSo(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		apptest.AnyMap{"full_name": "Greta Twice", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		apptest.AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	if status, _ := restore(t, e, "person", created.ID, entry.ID, readPerson(t, e, created.ID).Version); status != 200 {
		t.Fatalf("first restore → %d", status)
	}

	after := theEntryByID(t, readHistory(t, e, "person", created.ID), entry.ID)
	if after.Undoable.Undoable {
		t.Error("an entry a live reversal already covers still reads as undoable")
	} else if reasonOf(after) != "already_undone" {
		t.Errorf("reason = %v, want already_undone", reasonOf(after))
	}
	if status, _ := restore(t, e, "person", created.ID, entry.ID, readPerson(t, e, created.ID).Version); status != 409 {
		t.Errorf("second restore → %d, want 409", status)
	}
}

// A later write of the same field refuses the restore rather than clobbering
// it. Where another person edited in between the result is ambiguous, and
// saying so IS the behaviour.
func TestEndToEnd_aFieldWrittenAgainRefusesTheRestore(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		apptest.AnyMap{"full_name": "Greta Superseded", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, apptest.AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("first patch → %d", status)
	}
	target := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, apptest.AnyMap{"title": "COO"}, nil, nil); status != 200 {
		t.Fatalf("second patch → %d", status)
	}

	after := theEntryByID(t, readHistory(t, e, "person", created.ID), target.ID)
	if after.Undoable.Undoable {
		t.Fatal("an entry whose field was written again still reads as undoable")
	}
	if reasonOf(after) != "superseded" {
		t.Fatalf("reason = %v, want superseded", reasonOf(after))
	}
	if detailOf(after) != "title" {
		t.Errorf("detail = %v, want the field that moved", detailOf(after))
	}
	status, _ := restore(t, e, "person", created.ID, target.ID, readPerson(t, e, created.ID).Version)
	if status != 409 {
		t.Errorf("restoring a superseded entry → %d, want 409", status)
	}
}

// The dishonest-success case, at the only place it can be proved. activity's
// update path writes due_at as coalesce($n, due_at), so restoring the image's
// NULL would answer 200 and change nothing. It must refuse instead.
func TestEndToEnd_restoringNullIntoAColumnTheModuleCannotClearRefuses(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var activity struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/activities", apptest.AnyMap{
		"kind": "task", "subject": "Call Greta",
	}, nil, &activity); status != 201 {
		t.Fatalf("create activity → %d", status)
	}
	// due_at was empty; setting it records a before-image holding null.
	if status := e.Call(t, "PATCH", "/v1/activities/"+activity.ID,
		apptest.AnyMap{"due_at": "2026-09-01T10:00:00Z"}, nil, nil); status != 200 {
		t.Fatalf("set due_at → %d", status)
	}

	page := readHistory(t, e, "activity", activity.ID)
	entry := theUpdateEntry(t, page)
	if entry.Undoable.Undoable {
		t.Fatal("restoring a null into a coalesce-guarded column reads as undoable; " +
			"the write would answer success and change nothing")
	}
	if reasonOf(entry) != "null_unwritable_by_module" {
		t.Fatalf("reason = %v, want null_unwritable_by_module", reasonOf(entry))
	}
	if detailOf(entry) != "due_at" {
		t.Errorf("detail = %v, want the field that cannot be cleared", detailOf(entry))
	}
}

// The reversal is itself reversible, and reversing it REOPENS the original
// entry. Without this the trail is a one-way ratchet and an entry put back by
// mistake is stuck already_undone forever.
func TestEndToEnd_undoingAnUndoReopensTheOriginalEntry(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		apptest.AnyMap{"full_name": "Greta Reundo", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, apptest.AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	original := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	status, reversal := restore(t, e, "person", created.ID, original.ID, readPerson(t, e, created.ID).Version)
	if status != 200 {
		t.Fatalf("restore → %d", status)
	}

	status, _ = restore(t, e, "person", created.ID, reversal.ID, readPerson(t, e, created.ID).Version)
	if status != 200 {
		t.Fatalf("restoring the reversal → %d, want 200 — a restore row carries real images "+
			"by construction and is replayable", status)
	}
	if title := readPerson(t, e, created.ID).Title; title == nil || *title != "CEO" {
		t.Errorf("title after undoing the undo = %v, want the value the original change made", title)
	}
	reopened := theEntryByID(t, readHistory(t, e, "person", created.ID), original.ID)
	if !reopened.Undoable.Undoable {
		t.Errorf("the original entry stayed refused (%v) after its reversal was itself reversed; "+
			"the trail must be navigable in both directions", reasonOf(reopened))
	}
}

// A stale If-Match refuses and writes nothing. This is the whole concurrency
// argument: the binding evaluation runs just before the write, and every state
// change that could alter its answer bumps the record's version, so a decision
// taken on a stale reading cannot commit.
func TestEndToEnd_aStaleVersionRefusesTheRestoreAndWritesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		apptest.AnyMap{"full_name": "Greta Stale", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, apptest.AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	stale := readPerson(t, e, created.ID).Version

	// Somebody else touches the record between reading and pressing.
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		apptest.AnyMap{"full_name": "Greta Moved"}, nil, nil); status != 200 {
		t.Fatalf("concurrent patch → %d", status)
	}
	if status, _ := restore(t, e, "person", created.ID, entry.ID, stale); status != 409 {
		t.Errorf("restore on a stale version → %d, want 409", status)
	}
	moved := readPerson(t, e, created.ID)
	if moved.FullName != "Greta Moved" {
		t.Errorf("full_name = %q; the refused restore wrote anyway", moved.FullName)
	}
	if moved.Title == nil || *moved.Title != "CEO" {
		t.Errorf("title = %v; the refused restore wrote anyway", moved.Title)
	}
}

// An audit row belonging to a DIFFERENT record is 404, never 403. Telling a
// caller a row exists but is not theirs discloses the row.
func TestEndToEnd_anEntryFromAnotherRecordIsNotFound(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var mine, theirs personRecord
	if status := e.Call(t, "POST", "/v1/people", apptest.AnyMap{"full_name": "Greta Mine"}, nil, &mine); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/people", apptest.AnyMap{"full_name": "Greta Theirs", "title": "CTO"}, nil, &theirs); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+theirs.ID, apptest.AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	other := theUpdateEntry(t, readHistory(t, e, "person", theirs.ID))

	if status, _ := restore(t, e, "person", mine.ID, other.ID, readPerson(t, e, mine.ID).Version); status != 404 {
		t.Errorf("restoring another record's entry → %d, want 404", status)
	}
}

// reasonOf spells an entry's refusal, so a failure names the reason rather than
// the address of a pointer to it.
func reasonOf(entry historyEntry) string {
	if entry.Undoable.Reason == nil {
		return "<none>"
	}
	return *entry.Undoable.Reason
}

func detailOf(entry historyEntry) string {
	if entry.Undoable.Detail == nil {
		return "<none>"
	}
	return *entry.Undoable.Detail
}

// theEntryByID finds one line again after the record moved.
func theEntryByID(t *testing.T, page historyPage, id string) historyEntry {
	t.Helper()
	for _, entry := range page.Data {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("entry %s is no longer in the record's history", id)
	return historyEntry{}
}
