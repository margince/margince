// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Opening and patching a subject request over the handler. A request needs its
// deadline, and a patch tells an explicit null from a field it left out.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
		DueAt: new(time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("opening an assigned request: %v", err)
	}
	return row
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
	if w := e.patchDSR(t, request.ID, `{"status":"fulfilled","resolution":"package sent","identity_check":"id_document"}`); w.Code != http.StatusOK {
		t.Fatalf("fulfilling → %d: %s", w.Code, w.Body)
	}

	for _, blank := range []string{`null`, `""`, `"   "`} {
		w := e.patchDSR(t, request.ID, `{"resolution":`+blank+`}`)
		if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldResolution {
			t.Fatalf("resolution %s on a fulfilled request → %d %s, want 422 naming resolution", blank, w.Code, w.Body)
		}
	}
	if after := e.read(t, request.ID); after.Resolution == nil || *after.Resolution != "package sent" {
		t.Fatalf("a refused patch left the answer %v, want %q", after.Resolution, "package sent")
	}
}

func TestAClosedRequestRefusesAnotherAnswerAndAcceptsItsOwn(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "rewrite@dsr.test")
	if w := e.patchDSR(t, request.ID, `{"status":"rejected","resolution":"first answer","identity_check":"id_document"}`); w.Code != http.StatusOK {
		t.Fatalf("rejecting → %d: %s", w.Code, w.Body)
	}

	w := e.patchDSR(t, request.ID, `{"resolution":"another answer"}`)
	if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldResolution {
		t.Fatalf("a new answer on a rejected request → %d %s, want 422 naming resolution", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.Resolution == nil || *after.Resolution != "first answer" {
		t.Fatalf("a refused patch left the answer %v, want %q", after.Resolution, "first answer")
	}
	if w := e.patchDSR(t, request.ID, `{"resolution":"first answer"}`); w.Code != http.StatusOK {
		t.Fatalf("resending the stored answer → %d: %s", w.Code, w.Body)
	}
}

func TestARepeatedErasureFulfilmentIsAccepted(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindErasure, ids.NewV7().String())
	erase := func(context.Context, ids.UUID, string) (map[string]int64, error) { return map[string]int64{}, nil }
	in := UpdateDSRInput{Status: new("fulfilled"), Resolution: new("erased on request"), Proof: idDocumentChecked}

	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := e.store.FulfilErasure(e.ctx, request.ID, in, erase); err != nil {
			t.Fatalf("fulfilment attempt %d: %v", attempt, err)
		}
	}
	if after := e.read(t, request.ID); after.Resolution == nil || *after.Resolution != erasedResolution {
		t.Fatalf("the erased request holds %v, want %q", after.Resolution, erasedResolution)
	}
	if claims := e.auditedResolutionClaims(t, request.ID); len(claims) != 2 || claims[0] != "true" || claims[1] != "false" {
		t.Fatalf("the update rows claim the answer changed as %v, want [true false]: the repeat changed nothing", claims)
	}
}

// auditedResolutionClaims lists, oldest first, what each update row of the
// request says about its answer.
func (e *dsrEnv) auditedResolutionClaims(t *testing.T, id ids.UUID) []any {
	t.Helper()
	var claims []any
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(e.ctx, `
			SELECT after->'resolution' FROM audit_log
			 WHERE entity_type = 'data_subject_request' AND entity_id = $1 AND action = 'update'
			 ORDER BY occurred_at, id`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var claim []byte
			if err := rows.Scan(&claim); err != nil {
				return err
			}
			claims = append(claims, string(claim))
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the audit rows: %v", err)
	}
	return claims
}

func TestResendingTheStoredAnswerWritesNoAuditRow(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "audit-claim@dsr.test")
	if w := e.patchDSR(t, request.ID, `{"status":"fulfilled","resolution":"package sent","identity_check":"id_document"}`); w.Code != http.StatusOK {
		t.Fatalf("fulfilling → %d: %s", w.Code, w.Body)
	}
	if w := e.patchDSR(t, request.ID, `{"resolution":"package sent"}`); w.Code != http.StatusOK {
		t.Fatalf("resending the stored answer → %d: %s", w.Code, w.Body)
	}

	claims := e.auditedResolutionClaims(t, request.ID)
	if len(claims) != 1 || claims[0] != "true" {
		t.Fatalf("the update rows claim the answer changed as %v, want [true]: the repeat changed nothing and writes no row", claims)
	}
}

func TestClosingCannotBlankADraftAnswer(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "draft-close@dsr.test")
	if w := e.patchDSR(t, request.ID, `{"resolution":"draft answer"}`); w.Code != http.StatusOK {
		t.Fatalf("drafting an answer → %d: %s", w.Code, w.Body)
	}

	w := e.patchDSR(t, request.ID, `{"status":"rejected","resolution":"  "}`)
	if w.Code != http.StatusUnprocessableEntity || validationField(t, w.Body.Bytes()) != fieldResolution {
		t.Fatalf("closing over the draft with a blank answer → %d %s, want 422 naming resolution", w.Code, w.Body)
	}
	if after := e.read(t, request.ID); after.Status != "open" || after.Resolution == nil {
		t.Fatalf("a refused close left status %q and answer %v", after.Status, after.Resolution)
	}
}

// A key-share lock admits a plain read and the `UPDATE` but stops a `FOR
// UPDATE` read. A patch that waits on one validated the row it writes.
func TestAPatchValidatesTheRowItHolds(t *testing.T) {
	e := setupDSR(t)
	request := e.mustCreate(t, dsrKindAccess, "held@dsr.test")
	holder, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	release := func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the holder: %v", err)
		}
	}
	t.Cleanup(release)
	if _, err := holder.Exec(context.Background(),
		`SELECT 1 FROM data_subject_request WHERE id = $1 FOR KEY SHARE`, request.ID); err != nil {
		t.Fatalf("holding the request: %v", err)
	}

	bounded, cancel := context.WithTimeout(e.ctx, 250*time.Millisecond)
	defer cancel()
	if _, err := e.store.UpdateDSR(bounded, request.ID, UpdateDSRInput{ClearResolution: true}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a patch beside a held key-share lock answered %v, want it to wait: "+
			"its validating read took no row lock, so another close can land before its write", err)
	}
	release()
	if _, err := e.store.UpdateDSR(e.ctx, request.ID, UpdateDSRInput{ClearResolution: true}); err != nil {
		t.Fatalf("the patch once the lock is released: %v", err)
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
