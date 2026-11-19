// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// addLink drives the add-link handler the way the router does.
func addLink(t *testing.T, e *Env, h activities.Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, docWritePerms)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/attachments/links",
		strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.AddAttachmentLink(rec, req)
	return rec
}

func linkBody(company ids.UUID, url, extra string) string {
	return `{"entity_type":"company","entity_id":"` + company.String() + `","url":"` + url + `"` + extra + `}`
}

func TestAUserListsALinkOnACompanysDocuments(t *testing.T) {
	e := Setup(t)
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling)
	company := e.SeedCompany(t, "Acme", &e.Rep1)

	rec := addLink(t, e, h, linkBody(company, "  HTTPS://docs.google.com/document/d/abc/edit  ",
		`,"title":"Rollout\nplan","category":"offer"`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var link crmcontracts.Attachment
	if err := json.Unmarshal(rec.Body.Bytes(), &link); err != nil {
		t.Fatalf("decode link: %v", err)
	}
	if link.Url == nil || *link.Url != "https://docs.google.com/document/d/abc/edit" {
		t.Errorf("url = %v, want the trimmed address with a lower-case scheme", link.Url)
	}
	if link.Title == nil || *link.Title != "Rollout plan" {
		t.Errorf("title = %v, want one line", link.Title)
	}
	if link.Category == nil || string(*link.Category) != "offer" {
		t.Errorf("category = %v, want the one the user chose", link.Category)
	}
	if link.Source != "link" || link.BytesWithheld == nil || !*link.BytesWithheld {
		t.Errorf("source = %q, withheld = %v; want a link row with no bytes", link.Source, link.BytesWithheld != nil && *link.BytesWithheld)
	}
	if got := rec.Header().Get("Location"); got != "/v1/attachments/"+link.Id.String() {
		t.Errorf("Location = %q", got)
	}

	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, docWritePerms)
	listed, _, err := activities.NewStore(e.DB()).ListCompanyDocuments(ctx, company, activities.DocumentFilters{})
	if err != nil {
		t.Fatalf("listing the company's documents: %v", err)
	}
	if len(listed) != 1 || listed[0].Id != link.Id {
		t.Fatalf("documents = %v, want the link alone", listed)
	}

	// No bytes stand behind it, so a download finds nothing to serve.
	if _, _, err := activities.NewStore(e.DB()).OpenAttachment(ctx, ids.UUID(link.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("opening the link's bytes: err = %v, want not found", err)
	}

	// The user may refile it later, as they may an upload.
	out := decodeAttachment(t, patchMetadata(t, e, h, ids.UUID(link.Id), `{"category":"contract"}`))
	if out.Category == nil || string(*out.Category) != "contract" {
		t.Errorf("category after refiling = %v, want contract", out.Category)
	}
	rec = patchMetadata(t, e, h, ids.UUID(link.Id), `{"category":"link"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("refiling as a mailed link: status = %d, want 422", rec.Code)
	}
}

func TestALinkIsRefusedWhereItWouldMislead(t *testing.T) {
	e := Setup(t)
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	if rec := addLink(t, e, h, linkBody(company, "https://example.com/brief", "")); rec.Code != http.StatusCreated {
		t.Fatalf("first add: status = %d (body: %s)", rec.Code, rec.Body.String())
	}

	cases := map[string]struct{ body, code string }{
		"the same address twice":  {linkBody(company, "https://example.com/brief", ""), "link_already_listed"},
		"a provenance category":   {linkBody(company, "https://example.com/a", `,"category":"link"`), "category_not_assertable"},
		"a script address":        {linkBody(company, "javascript:alert(1)", ""), "invalid_url"},
		"a user name in the host": {linkBody(company, "https://bank.example@evil.example/", ""), "invalid_url"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := addLink(t, e, h, c.body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422 (body: %s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), c.code) {
				t.Errorf("body = %s, want %s", rec.Body.String(), c.code)
			}
		})
	}
}
