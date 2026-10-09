// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Opening and patching a subject request over the handler. A request needs its
// deadline, and a patch tells an explicit null from a field it left out.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dsrEnv) handlers() Handlers {
	return NewHandlers(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
}

// sendDSR drives one handler with a JSON body, as the router would.
func (e *dsrEnv) sendDSR(method, body string, serve func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/v1/data-subject-requests", strings.NewReader(body)).WithContext(e.ctx)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	serve(w, r)
	return w
}

func (e *dsrEnv) patchDSR(t *testing.T, id ids.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return e.sendDSR(http.MethodPatch, body, func(w http.ResponseWriter, r *http.Request) {
		e.handlers().UpdateDataSubjectRequest(w, r, openapi_types.UUID(id))
	})
}

func (e *dsrEnv) assigned(t *testing.T) dsrRow {
	t.Helper()
	officer := ids.From[ids.UserKind](e.user)
	row, err := e.store.CreateDSR(e.ctx, CreateDSRInput{
		Kind: dsrKindAccess, SubjectRef: "assigned@dsr.test", AssigneeID: &officer,
		DueAt: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("opening an assigned request: %v", err)
	}
	return row
}

func TestARequestWithoutADueDateIsRefused(t *testing.T) {
	e := setupDSR(t)
	for _, body := range []string{
		`{"kind":"access","subject_ref":"x@dsr.test"}`,
		`{"kind":"access","subject_ref":"x@dsr.test","due_at":null}`,
	} {
		w := e.sendDSR(http.MethodPost, body, func(w http.ResponseWriter, r *http.Request) {
			e.handlers().CreateDataSubjectRequest(w, r, crmcontracts.CreateDataSubjectRequestParams{})
		})
		if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldDueAt {
			t.Fatalf("%s answered %d %s, want 422 naming due_at: a request stored without one "+
				"sorts as overdue since the year 1", body, w.Code, w.Body)
		}
	}
	rows, _, err := e.store.ListDSRs(e.ctx, nil, "", "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("a refused request was stored anyway: %d rows, %v", len(rows), err)
	}
}

func TestANullAssigneeHandsTheRequestBack(t *testing.T) {
	e := setupDSR(t)
	request := e.assigned(t)

	if w := e.patchDSR(t, request.ID, `{"status":"in_progress"}`); w.Code != http.StatusOK {
		t.Fatalf("moving the request → %d: %s", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.AssigneeID == nil {
		t.Fatal("a patch that did not mention the assignee cleared it")
	}
	if w := e.patchDSR(t, request.ID, `{"assignee_id":null}`); w.Code != http.StatusOK {
		t.Fatalf("clearing the assignee → %d: %s", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.AssigneeID != nil {
		t.Fatalf("assignee_id null answered 200 and left %v assigned", *after.AssigneeID)
	}
	var before, after map[string]any
	if err := e.owner.QueryRow(context.Background(), `
		SELECT before, after FROM audit_log
		 WHERE entity_id = $1 AND action = 'update'
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, request.ID).Scan(&before, &after); err != nil {
		t.Fatalf("reading the hand-back's audit row: %v", err)
	}
	if before[fieldAssigneeID] != e.user.String() || after[fieldAssigneeID] != nil {
		t.Errorf("the hand-back audited assignee %v → %v, want %v → null: the row must say who let it go",
			before[fieldAssigneeID], after[fieldAssigneeID], e.user)
	}
	if _, recorded := after[fieldAssigneeID]; !recorded {
		t.Error("the hand-back's after image omits the assignee instead of recording it as null")
	}
}

func TestAFailedNameReadStillAnswersTheWriteAndTheQueue(t *testing.T) {
	e := setupDSR(t)
	h := e.handlers().WithRecordNames(failingNames{})
	created := e.sendDSR(http.MethodPost, `{"kind":"access","subject_ref":"`+e.user.String()+`","due_at":"2026-08-31T00:00:00Z"}`,
		func(w http.ResponseWriter, r *http.Request) {
			h.CreateDataSubjectRequest(w, r, crmcontracts.CreateDataSubjectRequestParams{})
		})
	if created.Code != http.StatusCreated {
		t.Fatalf("filing with a failing name read → %d %s, want 201: a 500 invites a duplicate statutory request",
			created.Code, created.Body)
	}
	listed := e.sendDSR(http.MethodGet, "", func(w http.ResponseWriter, r *http.Request) {
		h.ListDataSubjectRequests(w, r, crmcontracts.ListDataSubjectRequestsParams{})
	})
	var page struct {
		Data []crmcontracts.DataSubjectRequest `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); listed.Code != http.StatusOK || err != nil {
		t.Fatalf("listing with a failing name read → %d %s (%v), want the page", listed.Code, listed.Body, err)
	}
	if len(page.Data) != 1 || page.Data[0].SubjectLabel != nil || page.Data[0].SubjectKind != nil {
		t.Errorf("the queue answered %+v, want the one request with its subject unnamed", page.Data)
	}
}

func TestANullResolutionClearsAnOpenRequestsAnswer(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "draft@dsr.test")

	if w := e.patchDSR(t, request.ID, `{"resolution":"draft answer"}`); w.Code != http.StatusOK {
		t.Fatalf("drafting an answer → %d: %s", w.Code, w.Body)
	}
	if w := e.patchDSR(t, request.ID, `{"status":"in_progress"}`); w.Code != http.StatusOK {
		t.Fatalf("moving the request → %d: %s", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.Resolution == nil {
		t.Fatal("a patch that did not mention the resolution cleared it")
	}
	if w := e.patchDSR(t, request.ID, `{"resolution":null}`); w.Code != http.StatusOK {
		t.Fatalf("clearing the draft → %d: %s", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.Resolution != nil {
		t.Fatalf("resolution null answered 200 and left %q", *after.Resolution)
	}
}

func TestAClosedRequestKeepsItsAnswer(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "closed@dsr.test")
	if w := e.patchDSR(t, request.ID, `{"status":"fulfilled","resolution":"package sent"}`); w.Code != http.StatusOK {
		t.Fatalf("fulfilling → %d: %s", w.Code, w.Body)
	}

	w := e.patchDSR(t, request.ID, `{"resolution":null}`)
	if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldResolution {
		t.Fatalf("clearing a fulfilled request's answer → %d %s, want 422 naming resolution", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.Resolution == nil {
		t.Fatal("a refused clear emptied the answer anyway")
	}
}

func TestANullStatusIsRefusedRatherThanIgnored(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "status@dsr.test")

	w := e.patchDSR(t, request.ID, `{"status":null}`)
	if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldStatus {
		t.Fatalf("status null → %d %s, want 422 naming status: it is not nullable", w.Code, w.Body)
	}
}

func (e *dsrEnv) read(t *testing.T, id ids.UUID) dsrRow {
	t.Helper()
	row, err := e.store.GetDSR(e.ctx, id)
	if err != nil {
		t.Fatalf("reading the request back: %v", err)
	}
	return row
}
