// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Reversing a LINK through the record-history route, end to end over HTTP.
//
// The route is the one a record row uses, and the audit row's own entity_type
// is what decides the mechanism. That makes the two identities the path holds —
// the record whose history is being read, and the entry being reversed —
// different for the first time, and every case here is one consequence of that
// difference: the entry is admitted from both ends, the reversal's own line
// comes back on the response, the guard that serialises two reversers names the
// EDGE rather than either record, and the two refusals write nothing.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type relationshipRecord struct {
	ID      string  `json:"id"`
	Kind    string  `json:"kind"`
	Role    *string `json:"role"`
	Version int64   `json:"version"`
}

type relationshipList struct {
	Data []relationshipRecord `json:"data"`
}

// linkedPair is one employment, its two records, and the versions a caller
// reading either record's history would have in hand.
type linkedPair struct {
	person string
	org    string
	edge   relationshipRecord
}

// seedEmployment creates the person, the organization and the link between them
// through the product's own endpoints. Seeding the edge any other way would
// prove nothing about the audit row this reversal reads.
//
// seededEdgeRole is the role every edge here is created with — a constant, since
// no case varies it and the reversal does not read the role's value.
const seededEdgeRole = "cto"

func seedEmploymentOverHTTP(t *testing.T, e *apptest.AppEnv) linkedPair {
	t.Helper()
	role := seededEdgeRole
	var person personRecord
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Ada Employed"}, nil, &person); status != 201 {
		t.Fatalf("create person → %d", status)
	}
	var org struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/organizations",
		AnyMap{"display_name": "Employer GmbH"}, nil, &org); status != 201 {
		t.Fatalf("create organization → %d", status)
	}
	return linkedPair{person: person.ID, org: org.ID, edge: linkEdge(t, e, AnyMap{
		"kind": "employment", "person_id": person.ID, "organization_id": org.ID,
		"role": role, "source": "manual",
	})}
}

func linkEdge(t *testing.T, e *apptest.AppEnv, body AnyMap) relationshipRecord {
	t.Helper()
	var rel relationshipRecord
	if status := e.Call(t, "POST", "/v1/relationships", body, nil, &rel); status != 201 {
		t.Fatalf("create relationship → %d", status)
	}
	return rel
}

// theEdgeEntry is the newest history line about a link, found by the block the
// read sets on exactly those rows.
func theEdgeEntry(t *testing.T, page historyPage, action string) historyEntry {
	t.Helper()
	for _, entry := range page.Data {
		if entry.Edge != nil && entry.Action == action {
			return entry
		}
	}
	t.Fatalf("no %q edge line in a history of %d rows: %+v", action, len(page.Data), page.Data)
	return historyEntry{}
}

func reverseEntry(t *testing.T, e *apptest.AppEnv, entityType, id, auditID string, version int64) (int, historyEntry) {
	t.Helper()
	var entry historyEntry
	status := e.Call(t, "POST", restoreRoute(entityType, id, auditID),
		nil, ifMatch(version), &entry)
	return status, entry
}

// reverseRefusal reads the route's REFUSAL: the status and the machine code a
// client matches on, which the reversal's own history line has no field for.
func reverseRefusal(t *testing.T, e *apptest.AppEnv, entityType, id, auditID string, version int64) (int, string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "POST", restoreRoute(entityType, id, auditID),
		nil, ifMatch(version), &problem)
	return status, problem.Code
}

func restoreRoute(entityType, id, auditID string) string {
	return fmt.Sprintf("/v1/records/%s/%s/history/%s/restore", entityType, id, auditID)
}

// ifMatch is the header the route REQUIRES: the record's last-seen version.
func ifMatch(version int64) map[string]string {
	return map[string]string{"If-Match": fmt.Sprint(version)}
}

// liveEdgesOf is the person's un-archived links, which is how "the edge is
// gone" is observable to a client.
func liveEdgesOf(t *testing.T, e *apptest.AppEnv, person string) []relationshipRecord {
	t.Helper()
	var list relationshipList
	if status := e.Call(t, "GET", "/v1/relationships?person_id="+person, nil, nil, &list); status != 200 {
		t.Fatalf("list relationships → %d", status)
	}
	return list.Data
}

// The link goes back, and the response says which line recorded it going back.
//
// The response body is the assertion that matters. A reverse whose write
// commits and whose read-back looks for the reversal on the RECORD finds
// nothing, and the path answers "the record already holds these values, so
// there was nothing to put back" — a real write reported as a no-op, which no
// assertion about the edge alone would catch.
func TestEndToEnd_anEdgeIsUnlinkedByReversingTheLineThatMadeIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)

	entry := theEdgeEntry(t, readHistory(t, e, "person", pair.person), "create")
	if !entry.Undoable.Undoable {
		t.Fatalf("a fresh link reads as not undoable: %v", entry.Undoable.Reason)
	}
	person := readPerson(t, e, pair.person)

	status, reversal := reverseEntry(t, e, "person", pair.person, entry.ID, person.Version)
	if status != 200 {
		t.Fatalf("reverse → %d, want 200 (reason %v)", status, reversal.Undoable.Reason)
	}
	if reversal.ID == "" || reversal.UndidAuditLogID == nil || *reversal.UndidAuditLogID != entry.ID {
		t.Errorf("the reversal's own line came back as %+v; want the row that reverses %s",
			reversal, entry.ID)
	}
	if edges := liveEdgesOf(t, e, pair.person); len(edges) != 0 {
		t.Errorf("the link is still live after the reverse: %+v", edges)
	}
}

// The SAME link, reversed from the other end. An edge sits on both records'
// histories, so both must be able to act on it — and the anchor's rules apply
// whichever page the person was reading.
func TestEndToEnd_anEdgeIsReversibleFromTheOtherEndToo(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)

	entry := theEdgeEntry(t, readHistory(t, e, "organization", pair.org), "create")
	var org struct {
		Version int64 `json:"version"`
	}
	if status := e.Call(t, "GET", "/v1/organizations/"+pair.org, nil, nil, &org); status != 200 {
		t.Fatalf("read organization → %d", status)
	}

	status, reversal := reverseEntry(t, e, "organization", pair.org, entry.ID, org.Version)
	if status != 200 {
		t.Fatalf("reverse from the company → %d, want 200 (reason %v)", status, reversal.Undoable.Reason)
	}
	if reversal.UndidAuditLogID == nil || *reversal.UndidAuditLogID != entry.ID {
		t.Errorf("the reversal's line came back as %+v, want the row reversing %s", reversal, entry.ID)
	}
	if edges := liveEdgesOf(t, e, pair.person); len(edges) != 0 {
		t.Errorf("the link is still live: %+v", edges)
	}
}

// An entry somebody has already reversed says so, and says it on the LINK's row.
//
// The advisory answer is read per page from the reversals live on the page's own
// subjects. Asked about the record alone it finds none of a link's, so the page
// goes on offering "Put back" on a link already put back, and the change and its
// undo never collapse into one line.
func TestEndToEnd_aLinkAlreadyPutBackSaysSoOnTheNextRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)

	entry := theEdgeEntry(t, readHistory(t, e, "person", pair.person), "create")
	person := readPerson(t, e, pair.person)
	if status, reversal := reverseEntry(t, e, "person", pair.person, entry.ID, person.Version); status != 200 {
		t.Fatalf("the first reverse → %d (reason %v)", status, reversal.Undoable.Reason)
	}

	again := theEntryByID(t, readHistory(t, e, "person", pair.person), entry.ID)
	if again.Undoable.Undoable || again.Undoable.Reason == nil || *again.Undoable.Reason != "already_undone" {
		t.Errorf("the reversed link reads as %+v, want a refusal naming already_undone", again.Undoable)
	}
}

// An unlink refuses by name and writes NOTHING. Putting a removed link back is
// an un-archive, which no write path here performs — so the refusal says so
// rather than hiding behind a generic reason, and the trail is unchanged after it.
func TestEndToEnd_reversingAnUnlinkRefusesByNameAndWritesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)
	if status := e.Call(t, "DELETE", "/v1/relationships/"+pair.edge.ID, nil,
		map[string]string{"If-Match": fmt.Sprint(pair.edge.Version)}, nil); status != 200 {
		t.Fatalf("unlink → %d", status)
	}

	before := readHistory(t, e, "person", pair.person)
	entry := theEdgeEntry(t, before, "archive")
	if entry.Undoable.Undoable {
		t.Fatal("an unlink reads as reversible; putting a removed link back is an un-archive")
	}
	if entry.Undoable.Reason == nil || *entry.Undoable.Reason != "edge_relink_unsupported" {
		t.Errorf("the unlink's refusal is %v, want edge_relink_unsupported", entry.Undoable.Reason)
	}

	person := readPerson(t, e, pair.person)
	status, _ := reverseEntry(t, e, "person", pair.person, entry.ID, person.Version)
	if status != 409 {
		t.Errorf("reversing an unlink → %d, want 409", status)
	}
	if after := readHistory(t, e, "person", pair.person); len(after.Data) != len(before.Data) {
		t.Errorf("the refusal wrote a row: history went from %d to %d lines",
			len(before.Data), len(after.Data))
	}
	if edges := liveEdgesOf(t, e, pair.person); len(edges) != 0 {
		t.Errorf("the refused reverse re-linked the edge: %+v", edges)
	}
}

// A project's company is refused BY NAME whatever the verb, and writes nothing.
//
// The kind is deliberately outside the generic relationship surface: it needs
// write authority over the project ROW rather than the object grant, and a
// project must keep at least one company. A generic reverse reaching it could
// archive the last company on a project its caller cannot write.
func TestEndToEnd_reversingAProjectCompanyRefusesByNameAndWritesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var org struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/organizations",
		AnyMap{"display_name": "Client GmbH"}, nil, &org); status != 201 {
		t.Fatalf("create organization → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/projects", AnyMap{
		"name": "Joint rollout", "organization_id": org.ID, "source": "manual",
	}, nil, nil); status != 201 {
		t.Fatalf("create project → %d", status)
	}

	before := readHistory(t, e, "organization", org.ID)
	entry := theEdgeEntry(t, before, "create")
	if entry.Edge.Kind != "project_company" {
		t.Fatalf("the company's newest link is %q, want the project_company the project's create wrote", entry.Edge.Kind)
	}
	if entry.Undoable.Undoable {
		t.Fatal("a project's company reads as reversible through the generic path")
	}
	if entry.Undoable.Detail == nil || *entry.Undoable.Detail != "project_company" {
		t.Errorf("the refusal detail is %v, want the kind named", entry.Undoable.Detail)
	}

	if status := e.Call(t, "GET", "/v1/organizations/"+org.ID, nil, nil, &org); status != 200 {
		t.Fatalf("read organization → %d", status)
	}
	if status, _ := reverseEntry(t, e, "organization", org.ID, entry.ID, org.Version); status != 409 {
		t.Errorf("reversing a project's company → %d, want 409", status)
	}
	if after := readHistory(t, e, "organization", org.ID); len(after.Data) != len(before.Data) {
		t.Errorf("the refusal wrote a row: history went from %d to %d lines",
			len(before.Data), len(after.Data))
	}
}

// Reversing a CHANGE to a link replays what the link held, onto the link — not
// onto either record, which never carried the field at all.
func TestEndToEnd_reversingAnEdgeChangeReplaysWhatTheLinkHeld(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)
	var patched relationshipRecord
	if status := e.Call(t, "PATCH", "/v1/relationships/"+pair.edge.ID,
		AnyMap{"role": "coo"}, map[string]string{"If-Match": fmt.Sprint(pair.edge.Version)},
		&patched); status != 200 {
		t.Fatalf("patch the link → %d", status)
	}

	entry := theEdgeEntry(t, readHistory(t, e, "person", pair.person), "update")
	person := readPerson(t, e, pair.person)
	status, reversal := reverseEntry(t, e, "person", pair.person, entry.ID, person.Version)
	if status != 200 {
		t.Fatalf("reverse the change → %d, want 200 (reason %v)", status, reversal.Undoable.Reason)
	}

	edges := liveEdgesOf(t, e, pair.person)
	if len(edges) != 1 {
		t.Fatalf("the link count changed on a field replay: %+v", edges)
	}
	if edges[0].Role == nil || *edges[0].Role != "cto" {
		t.Errorf("role after the reverse = %v, want the value the link held before the change", edges[0].Role)
	}
}

// An edge write does not touch either record it joins, so neither record's
// version moves — which is why the caller's If-Match cannot be the guard here,
// and why the EDGE's own version is what the reverse pins.
//
// Held as its own assertion rather than left implicit: the day an edge write
// starts bumping the anchor, the reasoning above stops being true and this is
// what says so.
func TestEndToEnd_anEdgeReverseDoesNotMoveEitherRecordsVersion(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)

	entry := theEdgeEntry(t, readHistory(t, e, "person", pair.person), "create")
	before := readPerson(t, e, pair.person)
	if status, reversal := reverseEntry(t, e, "person", pair.person, entry.ID, before.Version); status != 200 {
		t.Fatalf("reverse → %d (reason %v)", status, reversal.Undoable.Reason)
	}
	if after := readPerson(t, e, pair.person); after.Version != before.Version {
		t.Errorf("the person's version moved from %d to %d on an edge write; the record's If-Match "+
			"would then be a guard, and this path pins the edge's version instead",
			before.Version, after.Version)
	}
}

// Two people reversing ONE link from opposite ends over HTTP. Exactly one lands.
//
// Both requests are honestly concurrent, so WHICH refusal the loser gets depends
// on where it was overtaken: the version moved under it, or the link was already
// gone. Both are 409, and what may never happen is both committing or the loser
// being told its entry does not exist.
//
// What this case does NOT establish is that the loser was overtaken INSIDE the
// window between its decision and its write — nothing here holds it there, so a
// run in which the two requests simply happened in sequence looks the same.
// TestAnEdgeReverseOvertakenInsideItsDecisionWindowRefuses (package compose)
// forces that overtake on a channel; this one is the end-to-end shape over the
// real route, including the two ends being different records.
func TestEndToEnd_twoReversesOfOneLinkFromOppositeEndsLeaveExactlyOne(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)

	fromPerson := theEdgeEntry(t, readHistory(t, e, "person", pair.person), "create")
	fromOrg := theEdgeEntry(t, readHistory(t, e, "organization", pair.org), "create")
	if fromPerson.ID != fromOrg.ID {
		t.Fatalf("the two ends name different entries for one link: %s vs %s", fromPerson.ID, fromOrg.ID)
	}
	person := readPerson(t, e, pair.person)
	var org struct {
		Version int64 `json:"version"`
	}
	if status := e.Call(t, "GET", "/v1/organizations/"+pair.org, nil, nil, &org); status != 200 {
		t.Fatalf("read organization → %d", status)
	}

	type outcome struct {
		status int
		reason string
	}
	results := make(chan outcome, 2)
	go func() {
		status, entry := reverseEntry(t, e, "person", pair.person, fromPerson.ID, person.Version)
		results <- outcome{status, reasonOf(entry)}
	}()
	go func() {
		status, entry := reverseEntry(t, e, "organization", pair.org, fromOrg.ID, org.Version)
		results <- outcome{status, reasonOf(entry)}
	}()

	var won int
	var loser outcome
	for range 2 {
		got := <-results
		if got.status == 200 {
			won++
			continue
		}
		loser = got
	}
	if won != 1 {
		t.Fatalf("%d of the two reverses committed; exactly one link may be removed once", won)
	}
	if loser.status != 409 {
		t.Errorf("the losing reverse answered %d (%s), want 409", loser.status, loser.reason)
	}
}

// A reverse decided from a screen the RECORD has moved under is refused, and the
// link is untouched.
//
// If-Match is required on this route alone and means "the history screen I
// decided from was current". An edge write never moves either record's version,
// so the edge's own version cannot answer that question — and a route whose
// guard binds on one branch and not the other means two different things
// depending on which row a person pressed Undo on.
func TestEndToEnd_reversingALinkFromAStaleRecordScreenIsRefusedAndWritesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pair := seedEmploymentOverHTTP(t, e)

	entry := theEdgeEntry(t, readHistory(t, e, "person", pair.person), "create")
	stale := readPerson(t, e, pair.person)
	// The record moves under the open screen, which is the whole premise.
	if status := e.Call(t, "PATCH", "/v1/people/"+pair.person,
		AnyMap{"title": "COO"}, nil, nil); status != 200 {
		t.Fatalf("move the person under the screen → %d", status)
	}

	status, code := reverseRefusal(t, e, "person", pair.person, entry.ID, stale.Version)
	if status != http.StatusConflict || code != "version_skew" {
		t.Errorf("a reverse decided from a stale screen → %d %q, want 409 version_skew", status, code)
	}
	if edges := liveEdgesOf(t, e, pair.person); len(edges) != 1 {
		t.Errorf("the link is %+v after a refused reverse; the record's If-Match is "+
			"required on this route and the edge branch has to honour it too", edges)
	}
}
