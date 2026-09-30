// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The REST door's refusals for the status read and the undo, as a client meets
// them: the status and code on the wire, not the engine's error values.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bulkRouter serves the three batch routes over the real handlers.
func bulkRouter(e *integration.Env) chi.Router {
	h := bulkHandlers{engine: bulkEngineFor(e)}
	batch := func(req *http.Request) openapi_types.UUID {
		return openapi_types.UUID(ids.MustParse(chi.URLParam(req, "id")))
	}
	r := chi.NewRouter()
	r.Get("/v1/bulk/{id}", func(w http.ResponseWriter, req *http.Request) { h.GetBulkChange(w, req, batch(req)) })
	r.Post("/v1/bulk/{id}/undo/preview", func(w http.ResponseWriter, req *http.Request) { h.PreviewBulkUndo(w, req, batch(req)) })
	r.Post("/v1/bulk/{id}/undo", func(w http.ResponseWriter, req *http.Request) {
		h.UndoBulkChange(w, req, batch(req), crmcontracts.UndoBulkChangeParams{})
	})
	return r
}

type problemDTO struct {
	Code string `json:"code"`
}

func serveBulk(t *testing.T, r chi.Router, req *http.Request) (int, problemDTO) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var problem problemDTO
	if rec.Code != http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
			t.Fatalf("a refusal that is not a problem body: %s", rec.Body)
		}
	}
	return rec.Code, problem
}

func TestTheBatchRoutesAnswerTheirRefusalsOnTheWire(t *testing.T) {
	e := integration.Setup(t)
	r := bulkRouter(e)
	moved, err := bulkEngineFor(e).Execute(e.Admin(), reassignTo(e.Rep2, seedBulkContacts(t, e, e.Rep1, 1)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	batch := moved.BatchId.String()
	post := func(path, body string) *http.Request {
		return httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(e.Admin())
	}

	if code, _ := serveBulk(t, r, httptest.NewRequest(http.MethodGet, "/v1/bulk/"+ids.NewV7().String(), nil).WithContext(e.Admin())); code != http.StatusNotFound {
		t.Errorf("GET of a batch that does not exist → %d, want 404", code)
	}
	if code, _ := serveBulk(t, r, post("/v1/bulk/"+ids.NewV7().String()+"/undo/preview", "")); code != http.StatusNotFound {
		t.Errorf("previewing the undo of a batch that does not exist → %d, want 404", code)
	}
	if code, _ := serveBulk(t, r, post("/v1/bulk/"+batch+"/undo", `{"confirm_token":`)); code == http.StatusOK {
		t.Error("an undo whose body does not parse ran")
	}
	if code, _ := serveBulk(t, r, post("/v1/bulk/"+batch+"/undo", `{}`)); code != http.StatusOK {
		t.Fatalf("the first undo → %d, want 200", code)
	}
	if code, problem := serveBulk(t, r, post("/v1/bulk/"+batch+"/undo/preview", "")); code != http.StatusConflict || problem.Code != "bulk_already_undone" {
		t.Errorf("previewing a second undo → %d %q, want 409 bulk_already_undone", code, problem.Code)
	}
	if code, _ := serveBulk(t, r, httptest.NewRequest(http.MethodGet, "/v1/bulk/"+batch, nil).WithContext(e.Admin())); code != http.StatusOK {
		t.Errorf("GET of the undone batch → %d, want 200", code)
	}
}

// A seat that may archive contacts but not read tags is told what came back,
// and never which tag did not.
func TestAnUndoNamesATagLeftBehindOnlyToAReaderOfTags(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	archiver := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"contact": {Create: true, Read: true, Update: true, Delete: true},
		},
		RowScope: principal.RowScopeTeam,
	})
	seeded := seedSurroundedContacts(t, e, 1)
	owner := ids.From[ids.UserKind](e.Rep1)
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(seeded[0].item.Id)),
		contacts.UpdateContactInput{OwnerID: &owner}); err != nil {
		t.Fatalf("handing the contact to the seat: %v", err)
	}
	seeded[0].item.Version = versionOf(t, e, "contact", ids.UUID(seeded[0].item.Id))
	archived, err := engine.Execute(archiver, archiveContacts(itemsOf(seeded)))
	if err != nil || archived.Changed != 1 {
		t.Fatalf("the seat's archive → %+v, %v", archived, err)
	}
	if _, err := collections.NewStore(e.DB()).ArchiveTag(e.Admin(), ids.From[ids.TagKind](seeded[0].tag)); err != nil {
		t.Fatalf("archiving the tag: %v", err)
	}

	undone, err := engine.Undo(archiver, ids.UUID(archived.BatchId), "")
	if err != nil || undone.Changed != 1 {
		t.Fatalf("Undo → %+v, %v", undone, err)
	}
	if undone.LeftBehind != nil && len(*undone.LeftBehind) != 0 {
		t.Errorf("the undo names %v to a seat that may not read tags", *undone.LeftBehind)
	}
	status, err := engine.Status(e.Admin(), ids.UUID(undone.BatchId))
	if err != nil || len(status.LeftBehind) != 1 || status.LeftBehind[0].Kind != crmcontracts.BulkLeftBehindKindTag {
		t.Errorf("an administrator reads %+v, %v; want the archived tag named", status.LeftBehind, err)
	}
}
