// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every kind of record that can be archived answers the same three ways once
// it is archived: a second archive is a no-op with the first archive's status
// and no audit or event row, even with the If-Match the first one used; an edit
// is 409 record_archived; and a caller who cannot see the record gets 404.
//
// The kinds are the archive operations of crm.yaml, so a new one fails here
// until it has a fixture.

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// editableWhileArchived names the archive operations whose record still takes
// an edit after it is archived, and why.
var editableWhileArchived = gatekit.Waive(map[string]string{
	"archiveTag": "an admin renames an archived tag to free its name before a restore, so its edit is not refused",
})

// archiveDoor is one archive operation, and the edit of the same record.
type archiveDoor struct {
	method, path       string
	okStatus           int
	ifMatch            bool
	editMethod, record string
	// editConflict is whether the edit declares the 409 an archived record answers.
	editConflict bool
}

// archivedSubject is one record a fixture made, ready to archive.
type archivedSubject struct {
	params      map[string]string
	record      string
	version     int64
	edit        AnyMap
	editHeaders map[string]string
}

// archiveDoorsOf reads every operation that archives a record: the operations
// named archive*, and a DELETE whose summary says it archives. The record's
// own path is the archive's, or the one above a POST …/archive.
func archiveDoorsOf(t *testing.T) map[string]archiveDoor {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading api/crm.yaml: %v", err)
	}
	doors := map[string]archiveDoor{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			archives := strings.HasPrefix(op.OperationID, "archive") ||
				(method == http.MethodDelete && strings.HasPrefix(op.Summary, "Archive"))
			if !archives {
				continue
			}
			record := strings.TrimSuffix(path, "/archive")
			editMethod := editMethodOf(doc.Paths.Find(record))
			doors[op.OperationID] = archiveDoor{
				method: method, path: path, okStatus: okStatusOf(t, op), ifMatch: declaresIfMatch(op),
				editMethod: editMethod, record: record,
				editConflict: editMethod != "" && doc.Paths.Find(record).GetOperation(editMethod).Responses.Value("409") != nil,
			}
		}
	}
	return doors
}

func okStatusOf(t *testing.T, op *openapi3.Operation) int {
	t.Helper()
	for code := range op.Responses.Map() {
		if status, err := strconv.Atoi(code); err == nil && status >= 200 && status < 300 {
			return status
		}
	}
	t.Fatalf("%s declares no 2xx", op.OperationID)
	return 0
}

func declaresIfMatch(op *openapi3.Operation) bool {
	for _, p := range op.Parameters {
		if p.Value != nil && p.Value.In == "header" && p.Value.Name == "If-Match" {
			return true
		}
	}
	return false
}

// editMethodOf is the method that edits the record at its own path, or ""
// when the record has no edit there.
func editMethodOf(item *openapi3.PathItem) string {
	switch {
	case item == nil:
		return ""
	case item.Patch != nil:
		return http.MethodPatch
	case item.Put != nil:
		return http.MethodPut
	}
	return ""
}

func TestEveryArchiveOperationHasAFixture(t *testing.T) {
	doors := archiveDoorsOf(t)
	for op := range doors {
		if _, ok := archiveFixtures[op]; !ok {
			t.Errorf("%s archives a record and has no fixture in archiveFixtures", op)
		}
	}
	for op := range archiveFixtures {
		if _, ok := doors[op]; !ok {
			t.Errorf("archiveFixtures names %s, which crm.yaml does not declare as an archive", op)
		}
	}
	for op, door := range doors {
		if door.editMethod == "" {
			continue
		}
		if !editableWhileArchived.Waived(t, op) && !door.editConflict {
			t.Errorf("the edit beside %s declares no 409, which it answers once the record is archived", op)
		}
	}
	editableWhileArchived.AssertAllMatched(t)
}

func TestEveryArchivedRecordAnswersTheSameWay(t *testing.T) {
	doors := archiveDoorsOf(t)
	e := archivingApp(t)
	ops := make([]string, 0, len(doors))
	for op := range doors {
		ops = append(ops, op)
	}
	slices.Sort(ops)
	for _, op := range ops {
		fixture, ok := archiveFixtures[op]
		if !ok {
			continue
		}
		t.Run(op, func(t *testing.T) {
			door := doors[op]
			subject := fixture(t, e)
			path := "/v1" + fillPath(door.path, subject.params)
			headers := map[string]string{}
			if door.ifMatch {
				headers["If-Match"] = strconv.FormatInt(subject.version, 10)
			}
			if status := e.Call(t, door.method, path, nil, headers, nil); status != door.okStatus {
				t.Fatalf("first archive → %d, want %d", status, door.okStatus)
			}
			before := tracesOf(t, e, subject.record)
			if status := e.Call(t, door.method, path, nil, headers, nil); status != door.okStatus {
				t.Fatalf("second archive → %d, want %d as the first", status, door.okStatus)
			}
			if after := tracesOf(t, e, subject.record); after != before {
				t.Errorf("second archive wrote audit or event rows: %d → %d", before, after)
			}
			if door.editMethod != "" {
				assertEditAnswer(t, e, op, door.editMethod, "/v1"+fillPath(door.record, subject.params), subject)
			}
		})
	}
}

func assertEditAnswer(t *testing.T, e *apptest.AppEnv, op, method, path string, subject archivedSubject) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, method, path, subject.edit, subject.editHeaders, &problem)
	if slices.Contains(editableWhileArchived.Subjects(), op) {
		if status < 200 || status > 299 {
			t.Errorf("edit of the archived record → %d, want 2xx: it is editable while archived", status)
		}
		return
	}
	if status != http.StatusConflict || problem.Code != "record_archived" {
		t.Errorf("edit of the archived record → %d %q, want 409 record_archived", status, problem.Code)
	}
}

func TestAnArchivedRecordTheCallerCannotSeeIsNotFound(t *testing.T) {
	e := archivingApp(t)
	contact := madeContact(t, e).ID
	mustCall(t, e, "PATCH", "/v1/contacts/"+contact, AnyMap{"visibility": "owner"}, http.StatusOK, nil)
	mustCall(t, e, "DELETE", "/v1/contacts/"+contact, nil, http.StatusOK, nil)

	var role struct {
		Key string `json:"key"`
	}
	mustCall(t, e, "POST", "/v1/roles", AnyMap{"copy_from": "rep", "name": "Own contacts"}, http.StatusCreated, &role)
	mustCall(t, e, "PATCH", "/v1/roles/"+role.Key, AnyMap{"row_scope": "own"}, http.StatusOK, nil)
	const email, password = "rep@archived-answer.test", "archived-answer-rep-password"
	rep := createdRecord(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Rae Rep", "role": role.Key})
	apptest.SetPasswordByLink(t, e, rep, password)
	seat := e.AsSecondSeat(t, email, password)
	if status := seat.Call(t, "GET", "/v1/contacts/"+contact, nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("the rep read a colleague's contact → %d, want it hidden from them", status)
	}

	for _, call := range []struct{ method, label string }{{"PATCH", "edit"}, {"DELETE", "archive"}} {
		if status := seat.Call(t, call.method, "/v1/contacts/"+contact, AnyMap{"full_name": "Seen"}, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s of a colleague's archived contact → %d, want 404", call.label, status)
		}
	}
}

// archivingApp is an app that serves every kind of record a client can archive.
func archivingApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	cipher, err := webhooks.NewCipher(bytes.Repeat([]byte{0x5a}, webhooks.WebhookKeyBytes))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithListsEnabled(true),
		compose.WithBlobstore(blobstore.NewMemory()), compose.WithWebhookSigningKey(cipher))
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	return e
}

// tracesOf counts the audit and outbox rows that name record.
func tracesOf(t *testing.T, e *apptest.AppEnv, record string) int {
	t.Helper()
	var n int
	err := e.Pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_log WHERE entity_id::text = $1 OR after::text LIKE '%' || $1 || '%')
		     + (SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%')`, record).Scan(&n)
	if err != nil {
		t.Fatalf("counting the rows that name %s: %v", record, err)
	}
	return int(n)
}

func fillPath(path string, params map[string]string) string {
	for name, value := range params {
		path = strings.ReplaceAll(path, "{"+name+"}", value)
	}
	return path
}
