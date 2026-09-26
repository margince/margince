// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The importer's door on the four RECORD wires: contact, company, deal and
// project, driven over HTTP like importernamespace_integration_test drives
// activities and leads.
//
// A record import that cannot stamp mirror: on these wires lands nothing at
// all: the importer sends the namespace on every record it writes, and each
// wire answered 422 reserved_source_system for every caller until the handler
// learned to ask auth.DeclaredImporter. The mapper tests hold the admission;
// only this file shows each handler actually asks.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
)

// serveCreate drives one create handler as `as` and answers the status and the
// created row's id (empty unless 201/200).
func serveCreate(as context.Context, t *testing.T, path string, raw []byte,
	serve func(http.ResponseWriter, *http.Request),
) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	serve(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)).WithContext(as))
	var out struct {
		ID string `json:"id"`
	}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding %s: %v", path, err)
		}
	}
	return rec.Code, out.ID
}

type recordWire struct {
	table string
	path  string
	body  []byte
	serve func(http.ResponseWriter, *http.Request)
}

// recordWires builds one namespaced create body per record wire. A deal needs
// a pipeline and stage, and a project a company, so both are seeded first.
func recordWires(t *testing.T, e *integration.Env) []recordWire {
	t.Helper()
	admin := e.Admin()
	dealsStore := deals.NewStore(e.DB(), DealsInstallation())
	if err := dealsStore.SeedDefaults(admin); err != nil {
		t.Fatalf("seed default pipeline: %v", err)
	}
	pipeline, err := dealsStore.DefaultPipeline(admin)
	if err != nil || pipeline.Stages == nil || len(*pipeline.Stages) == 0 {
		t.Fatalf("default pipeline: %v", err)
	}
	company := openapi_types.UUID(e.SeedCompany(t, "Importer Anchor GmbH", nil))
	system := "mirror:hubspot"
	encode := func(body []byte, err error) []byte {
		if err != nil {
			t.Fatalf("encoding a create body: %v", err)
		}
		return body
	}
	c := contacts.NewHandlers(InstallationDB(e.Pool))
	d := deals.NewHandlers(e.DB(), DealsInstallation())
	p := projects.HandlersOver(ProjectsStore(e.Pool))
	return []recordWire{
		{
			"contact", "/v1/contacts", encode(json.Marshal(crmcontracts.CreateContactRequest{FullName: "Imported Contact", SourceSystem: &system})),
			func(w http.ResponseWriter, r *http.Request) {
				c.CreateContact(w, r, crmcontracts.CreateContactParams{})
			},
		},
		{
			"company", "/v1/companies", encode(json.Marshal(crmcontracts.CreateCompanyRequest{DisplayName: "Imported Company", SourceSystem: &system})),
			func(w http.ResponseWriter, r *http.Request) {
				c.CreateCompany(w, r, crmcontracts.CreateCompanyParams{})
			},
		},
		{"deal", "/v1/deals", encode(json.Marshal(crmcontracts.CreateDealRequest{
			Name: "Imported Deal", PipelineId: pipeline.Id, StageId: (*pipeline.Stages)[0].Id, SourceSystem: &system,
		})), func(w http.ResponseWriter, r *http.Request) { d.CreateDeal(w, r, crmcontracts.CreateDealParams{}) }},
		{"project", "/v1/projects", encode(json.Marshal(crmcontracts.CreateProjectRequest{
			Name: "Imported Project", CompanyId: company, Source: "ui", SourceSystem: &system,
		})), func(w http.ResponseWriter, r *http.Request) {
			p.CreateProject(w, r, crmcontracts.CreateProjectParams{})
		}},
	}
}

func TestADeclaredImporterLandsEveryRecordInsideItsNamespace(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.AdminUser, nil, importerPerms())
	for _, w := range recordWires(t, e) {
		status, id := serveCreate(importer, t, w.path, w.body, w.serve)
		if status != http.StatusCreated {
			t.Errorf("%s answered %d, want 201: the importer may stamp its own namespace", w.table, status)
			continue
		}
		if got := e.WsScalar(t, `SELECT source_system FROM `+w.table+` WHERE id = $1`, id); got != "mirror:hubspot" {
			t.Errorf("%s.source_system = %q, want mirror:hubspot", w.table, got)
		}
	}
}

// Same bodies, two callers who are not a declared importer: the admin's AGENT
// carrying the identical grants, and a human without import_run:create. Every
// wire must still refuse both, or the door leaked past the handler's question.
func TestTheRecordWiresStayClosedToEveryoneElse(t *testing.T) {
	e := integration.Setup(t)
	callers := map[string]context.Context{
		"agent with the grant":    e.AgentFor(t, e.AdminUser, nil, importerPerms()),
		"human without the grant": e.As(e.AdminUser, nil, integration.AdminPerms),
	}
	for _, w := range recordWires(t, e) {
		for who, as := range callers {
			if status, _ := serveCreate(as, t, w.path, w.body, w.serve); status != http.StatusUnprocessableEntity {
				t.Errorf("%s as %s answered %d, want 422 reserved_source_system", w.table, who, status)
			}
		}
	}
}
