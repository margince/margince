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
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type historyEntry struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
	// UndidAuditLogID is the entry this line REVERSES, which is how a reversal is
	// told from a fresh change.
	UndidAuditLogID *string `json:"undid_audit_log_id"`
	// Edge is set exactly on the lines that changed a LINK rather than a field.
	Edge     *historyEdge `json:"edge"`
	Undoable struct {
		Undoable bool    `json:"undoable"`
		Reason   *string `json:"reason"`
		Detail   *string `json:"detail"`
	} `json:"undoable"`
	Before   map[string]any `json:"before"`
	After    map[string]any `json:"after"`
	Evidence map[string]any `json:"evidence"`
}

type historyEdge struct {
	Kind            string  `json:"kind"`
	OtherEntityType string  `json:"other_entity_type"`
	OtherEntityID   string  `json:"other_entity_id"`
	OtherLabel      *string `json:"other_label"`
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

// readHistory returns the record's history, NEWEST FIRST — the order the
// projection serves and the order every reader in this file assumes — with each
// entry's undoability as the surface would show it.
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
	// Newest first, so the newest update is the first one encountered.
	for _, entry := range page.Data {
		if entry.Action == "update" {
			return entry
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
		AnyMap{"full_name": "Greta Original", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"title": "CEO"}, nil, nil); status != 200 {
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
		AnyMap{"full_name": "Greta Twice", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"title": "CEO"}, nil, nil); status != 200 {
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
		AnyMap{"full_name": "Greta Superseded", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("first patch → %d", status)
	}
	target := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{"title": "COO"}, nil, nil); status != 200 {
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
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "task", "subject": "Call Greta",
	}, nil, &activity); status != 201 {
		t.Fatalf("create activity → %d", status)
	}
	// due_at was empty; setting it records a before-image holding null.
	if status := e.Call(t, "PATCH", "/v1/activities/"+activity.ID,
		AnyMap{"due_at": "2026-09-01T10:00:00Z"}, nil, nil); status != 200 {
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
		AnyMap{"full_name": "Greta Reundo", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{"title": "CEO"}, nil, nil); status != 200 {
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
		AnyMap{"full_name": "Greta Stale", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	stale := readPerson(t, e, created.ID).Version

	// Somebody else touches the record between reading and pressing.
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"full_name": "Greta Moved"}, nil, nil); status != 200 {
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
	if status := e.Call(t, "POST", "/v1/people", AnyMap{"full_name": "Greta Mine"}, nil, &mine); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/people", AnyMap{"full_name": "Greta Theirs", "title": "CTO"}, nil, &theirs); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+theirs.ID, AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	other := theUpdateEntry(t, readHistory(t, e, "person", theirs.ID))

	if status, _ := restore(t, e, "person", mine.ID, other.ID, readPerson(t, e, mine.ID).Version); status != 404 {
		t.Errorf("restoring another record's entry → %d, want 404", status)
	}
}

// newUUID is a well-formed id that names nothing, for the paths that must be
// refused before anything is looked up.
func newUUID() string { return ids.NewV7().String() }

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

// An archived record's update path refuses on its own terms. Naming it here
// makes the refusal legible instead of a surprise the person reads as a bug.
func TestEndToEnd_anArchivedRecordSaysSoRatherThanFailingLater(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Greta Archived", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	version := readPerson(t, e, created.ID).Version
	if status := e.Call(t, "DELETE", "/v1/people/"+created.ID, nil, nil, nil); status != 200 && status != 204 {
		t.Fatalf("archive → %d", status)
	}

	if status, _ := restore(t, e, "person", created.ID, entry.ID, version); status == 200 {
		t.Error("restoring an archived record answered 200")
	}
}

// A restore whose image names a custom field the workspace has since retired
// cannot be written back. Without this refusal the module's own "unknown field
// cf_budget" is what a person reads, and that is not an answer to pressing Undo.
func TestEndToEnd_aRetiredCustomFieldRefusesByNamingIt(t *testing.T) {
	// The schema pool is what lets a custom field actually add its column;
	// without it the catalog route answers 501 and the case cannot be built.
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(SchemaPool(t)))
	e.BootstrapWorkspace(t)

	var field struct {
		ID         string `json:"id"`
		ColumnName string `json:"column_name"`
		Version    int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/custom-fields", AnyMap{
		"object": "person", "label": "Budget", "type": "text", "source": "manual",
	}, nil, &field); status != 201 {
		t.Fatalf("create custom field → %d", status)
	}

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": "Greta Custom", field.ColumnName: "first",
	}, nil, &created); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{field.ColumnName: "second"}, nil, nil); status != 200 {
		t.Fatalf("patch custom field → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	if !entry.Undoable.Undoable {
		t.Fatalf("a live custom field's change reads as not undoable: %s", reasonOf(entry))
	}

	if status := e.Call(t, "POST", "/v1/custom-fields/"+field.ID+"/retire",
		nil, nil, nil); status != 200 {
		t.Fatalf("retire the field → %d", status)
	}

	after := theEntryByID(t, readHistory(t, e, "person", created.ID), entry.ID)
	if after.Undoable.Undoable {
		t.Fatal("a change to a retired custom field still reads as undoable; the write " +
			"would fail inside the module with a message written for another surface")
	}
	if reasonOf(after) != "not_restorable_by_this_path" {
		t.Errorf("reason = %s, want not_restorable_by_this_path", reasonOf(after))
	}
	if detailOf(after) != field.ColumnName {
		t.Errorf("detail = %s, want the field that cannot be written back", detailOf(after))
	}
}

// A record type the history screens do not serve is refused as a validation
// error, not carried into the seam. The path parameter is a bare string the
// generator does not validate, so this handler is its enforcement point.
func TestEndToEnd_anUnservedRecordTypeIsRefusedAtTheRoute(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	if status, _ := restore(t, e, "relationship", newUUID(), newUUID(), 1); status != 422 {
		t.Errorf("restoring a relationship → %d, want 422", status)
	}
}

// If-Match is required here and must be a version. A restore is decided from a
// screen the person has been reading, so a missing or unparseable precondition
// is a refusal rather than a last-write-wins default.
func TestEndToEnd_aRestoreWithoutAUsableIfMatchIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Greta Precondition", "title": "CTO"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{"title": "CEO"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	path := fmt.Sprintf("/v1/records/person/%s/history/%s/restore", created.ID, entry.ID)

	if status := e.Call(t, "POST", path, nil, nil, nil); status == 200 {
		t.Error("a restore with no If-Match answered 200; the precondition is required here")
	}
	if status := e.Call(t, "POST", path, nil, map[string]string{"If-Match": "not-a-version"}, nil); status != 422 {
		t.Errorf("a restore with an unparseable If-Match → %d, want 422", status)
	}
}

// The window the version guard cannot see.
//
// Retiring a custom field writes `custom_field` and not the record, so the
// record's version does not move. A restore decided while the field was live
// therefore passes If-Match after it is retired — the one precondition that
// closes every other such gap cannot see this one.
//
// What catches it is that the BINDING evaluation reads the live catalog at
// write time rather than trusting the reading the screen was decided from.
// This test holds that, and the assertion on the version is what stops it
// becoming vacuous if a retire ever starts touching the record.
func TestEndToEnd_aRetireTheVersionGuardCannotSeeIsCaughtAtWriteTime(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(SchemaPool(t)))
	e.BootstrapWorkspace(t)

	var field struct {
		ID         string `json:"id"`
		ColumnName string `json:"column_name"`
	}
	if status := e.Call(t, "POST", "/v1/custom-fields", AnyMap{
		"object": "person", "label": "Budget", "type": "text", "source": "manual",
	}, nil, &field); status != 201 {
		t.Fatalf("create custom field → %d", status)
	}
	var created personRecord
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": "Greta Dropped", field.ColumnName: "first",
	}, nil, &created); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{field.ColumnName: "second"}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}
	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	// The version a person reading the screen would hold, taken while the
	// field is still live and the entry still reads as undoable.
	decided := readPerson(t, e, created.ID).Version

	if status := e.Call(t, "POST", "/v1/custom-fields/"+field.ID+"/retire", nil, nil, nil); status != 200 {
		t.Fatalf("retire → %d", status)
	}
	if now := readPerson(t, e, created.ID).Version; now != decided {
		t.Fatalf("the retire moved the record's version from %d to %d; the window this "+
			"test exists for is not real any more and the test proves nothing",
			decided, now)
	}

	status, _ := restore(t, e, "person", created.ID, entry.ID, decided)
	if status == 200 {
		t.Error("a restore of a since-retired field answered 200; the person is told a " +
			"change was put back that the write dropped")
	}
	if status != 409 {
		t.Errorf("restore → %d, want 409", status)
	}
}

// Filling a field in and then putting it back to empty — the case a person
// most often reaches for undo on, and the one a JSON null cannot express.
//
// The before-image holds a null, so the restore must ask for the field to be
// CLEARED rather than send the null: an optional pointer decodes a null as "not
// supplied", and the write would report success having changed nothing.
func TestEndToEnd_aFieldFilledInGoesBackToEmpty(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// Created with NO title, so the change below records a before-image of null.
	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Greta Cleared"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"title": "Typed by mistake"}, nil, nil); status != 200 {
		t.Fatalf("fill the field → %d", status)
	}

	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	if !entry.Undoable.Undoable {
		t.Fatalf("filling an empty field reads as not undoable (%s); this is the case a "+
			"person reaches for undo on most", reasonOf(entry))
	}

	status, _ := restore(t, e, "person", created.ID, entry.ID, readPerson(t, e, created.ID).Version)
	if status != 200 {
		t.Fatalf("restore → %d, want 200", status)
	}
	if title := readPerson(t, e, created.ID).Title; title != nil && *title != "" {
		t.Errorf("title after the restore = %q, want it empty again — the restore "+
			"reported success and left the value standing", *title)
	}
	// The trail says what happened, so the clear is auditable rather than a
	// silent write.
	// The page is newest first, so the reversal is the FIRST line.
	page := readHistory(t, e, "person", created.ID)
	reversal := page.Data[0]
	if reversal.Action != "restore" {
		t.Fatalf("the newest entry is %q, want restore", reversal.Action)
	}
	if _, recorded := reversal.After["title"]; !recorded {
		t.Errorf("the reversal's image does not mention title: %v", reversal.After)
	}
}

// An activity cannot clear, and says so. Its update statement writes every
// column as coalesce($n, col), so the placeholder's NULL selects the current
// value and no argument can clear one.
func TestEndToEnd_anActivityFieldFilledInRefusesBecauseItCannotBeCleared(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var activity struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "task", "subject": "Call Greta",
	}, nil, &activity); status != 201 {
		t.Fatalf("create activity → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/activities/"+activity.ID,
		AnyMap{"due_at": "2026-09-01T10:00:00Z"}, nil, nil); status != 200 {
		t.Fatalf("set due_at → %d", status)
	}

	entry := theUpdateEntry(t, readHistory(t, e, "activity", activity.ID))
	if entry.Undoable.Undoable {
		t.Fatal("an activity field that cannot be cleared reads as undoable; the write " +
			"would answer success and change nothing")
	}
	if reasonOf(entry) != "null_unwritable_by_module" {
		t.Fatalf("reason = %s, want null_unwritable_by_module", reasonOf(entry))
	}
	if detailOf(entry) != "due_at" {
		t.Errorf("detail = %s, want the field it cannot clear", detailOf(entry))
	}
}

// An address arrives as six columns and goes back as one nested object. Without
// the fold every key reads as unspellable and an edit that touched an address
// would be permanently un-undoable.
func TestEndToEnd_anAddressGoesBackAsOneField(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": "Greta Address",
		"address":   AnyMap{"city": "Hanoi", "line1": "1 First Street"},
	}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID, AnyMap{
		"address": AnyMap{"city": "Da Nang", "line1": "2 Second Street"},
	}, nil, nil); status != 200 {
		t.Fatalf("change the address → %d", status)
	}

	entry := theUpdateEntry(t, readHistory(t, e, "person", created.ID))
	if !entry.Undoable.Undoable {
		t.Fatalf("an address change reads as not undoable (%s / %s)",
			reasonOf(entry), detailOf(entry))
	}
	status, _ := restore(t, e, "person", created.ID, entry.ID, readPerson(t, e, created.ID).Version)
	if status != 200 {
		t.Fatalf("restore → %d, want 200", status)
	}

	var back struct {
		Address *struct {
			City  *string `json:"city"`
			Line1 *string `json:"line1"`
		} `json:"address"`
	}
	if status := e.Call(t, "GET", "/v1/people/"+created.ID, nil, nil, &back); status != 200 {
		t.Fatalf("read back → %d", status)
	}
	if back.Address == nil || back.Address.City == nil || *back.Address.City != "Hanoi" {
		t.Errorf("address.city after the restore = %v, want Hanoi", back.Address)
	}
}

// A -> B -> C, reverted C -> B -> A.
//
// This is what a person means by undo: walk back through the record's history
// one change at a time. It works only because supersession asks whether the
// field's VALUE has moved rather than whether anybody wrote it — undoing C
// writes B and records a reversal row, and a rule that counted writes would see
// that row as a later write of the same field and refuse the B entry, so the
// walk could never get past its first step.
func TestEndToEnd_severalChangesGoBackOneAtATime(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Walker", "title": "A"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	for _, value := range []string{"B", "C"} {
		if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
			AnyMap{"title": value}, nil, nil); status != 200 {
			t.Fatalf("set title %s → %d", value, status)
		}
	}
	if title := readPerson(t, e, created.ID).Title; title == nil || *title != "C" {
		t.Fatalf("title = %v, want C before the walk starts", title)
	}

	// Walk back twice. Each step takes the newest entry that is undoable, which
	// is what pressing the top "Put back" does.
	for _, want := range []string{"B", "A"} {
		page := readHistory(t, e, "person", created.ID)
		// The newest undoable ORIGINAL change. A reversal row is undoable too,
		// but pressing its button REDOES the change it reversed — that is the
		// other direction, not the next step back.
		var target string
		for _, entry := range page.Data {
			if entry.Undoable.Undoable && entry.Action == "update" {
				target = entry.ID
				break
			}
		}
		if target == "" {
			t.Fatalf("no undoable entry while walking back to %s; the walk stopped early:\n%s",
				want, undoabilityOf(page))
		}
		status, _ := restore(t, e, "person", created.ID, target, readPerson(t, e, created.ID).Version)
		if status != 200 {
			t.Fatalf("restore toward %s → %d", want, status)
		}
		title := readPerson(t, e, created.ID).Title
		if title == nil {
			t.Fatalf("title after the step is empty, want %s", want)
		}
		if *title != want {
			t.Fatalf("title after the step = %q, want %s", *title, want)
		}
	}
}

// undoabilityOf spells a page's verdicts, so a walk that stopped early says
// which entry refused and why rather than only that it stopped.
func undoabilityOf(page historyPage) string {
	var out strings.Builder
	for _, entry := range page.Data {
		fmt.Fprintf(&out, "\t%s %s: undoable=%v %s %s\n",
			entry.Action, entry.ID, entry.Undoable.Undoable, reasonOf(entry), detailOf(entry))
	}
	return out.String()
}

// An explicit null on a nullable field CLEARS it, and does not answer success
// having changed nothing.
//
// The contract declares these fields `[string, 'null']`, so a null is a request
// the server promised to honour. The decoded pointer cannot tell it from an
// absent field, which is why it used to be dropped — a 200 the caller could not
// trust, on the public API and nothing to do with undo.
func TestEndToEnd_anExplicitNullClearsTheFieldRatherThanBeingIgnored(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Nullable", "title": "Head of Nothing"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"title": nil}, nil, nil); status != 200 {
		t.Fatalf("clear the title → %d", status)
	}
	if title := readPerson(t, e, created.ID).Title; title != nil && *title != "" {
		t.Errorf("title = %q after an explicit null; the field was not cleared and the "+
			"caller was told it was", *title)
	}

	// An absent field is still left alone — the whole point of telling the two
	// apart. A patch of one field must not wipe the others.
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"title": "Restored By Hand"}, nil, nil); status != 200 {
		t.Fatalf("set the title again → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"first_name": "Nully"}, nil, nil); status != 200 {
		t.Fatalf("patch a different field → %d", status)
	}
	if title := readPerson(t, e, created.ID).Title; title == nil || *title != "Restored By Hand" {
		t.Errorf("title = %v after patching another field; an absent field was treated "+
			"as a clear", title)
	}
}

// A null on a field this record cannot clear is REFUSED by name, not dropped.
func TestEndToEnd_aNullOnAnUnclearableFieldIsRefusedByName(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Named Forever"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	// full_name is not nullable in the contract and a record with no name is
	// not a record anybody can find again.
	if status := e.Call(t, "PATCH", "/v1/people/"+created.ID,
		AnyMap{"full_name": nil}, nil, nil); status != 422 {
		t.Errorf("clearing full_name → %d, want 422 naming the field", status)
	}
	if name := readPerson(t, e, created.ID).FullName; name != "Named Forever" {
		t.Errorf("full_name = %q; the refused clear wrote anyway", name)
	}
}

// An entry whose image mentions a field the record keeps in its own table is
// still undoable.
//
// Supersession compares the image against the row, and a company's domains and
// relationship types are not columns on it — so comparing them read every such
// entry as "somebody changed these fields since" when nobody had. The newest
// entry on a record refused, which is the one a person is most likely to want.
func TestEndToEnd_anEntryTouchingAFieldHeldElsewhereIsStillUndoable(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/organizations",
		AnyMap{"display_name": "Held Elsewhere Ltd"}, nil, &created); status != 201 {
		t.Fatalf("create → %d", status)
	}
	// domains and relationship_types live in their own tables; industry is an
	// ordinary column, so the entry mixes both kinds.
	if status := e.Call(t, "PATCH", "/v1/organizations/"+created.ID, AnyMap{
		"industry":           "Manufacturing",
		"domains":            []AnyMap{{"domain": "held.test", "is_primary": true}},
		"relationship_types": []string{"customer"},
	}, nil, nil); status != 200 {
		t.Fatalf("patch → %d", status)
	}

	page := readHistory(t, e, "organization", created.ID)
	entry := theUpdateEntry(t, page)
	if !entry.Undoable.Undoable {
		t.Fatalf("the newest entry refused as %q (%q); nothing was written after it, and a "+
			"field the row does not hold as a column cannot have moved",
			reasonOf(entry), detailOf(entry))
	}
}
