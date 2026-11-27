// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A filter that reaches no SQL still answers a well-formed page — of the wrong
// rows. That is the one failure the whole list_records path is built against,
// and it is the one no unit test can see: the schema check passes, the envelope
// passes, the seam is called with the filter in hand, and the rows come back
// unnarrowed because nothing bound it to a predicate.
//
// So the assertion here is exclusion, against a real database: two records that
// differ ONLY in the filtered field, and a list that must return one of them.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestListRecordsNarrowsRatherThanAnsweringEveryRow(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	inStage := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"deal","fields":{"name":"Narrowed in","pipeline_id":"`+
			pipeline.String()+`","stage_id":"`+open.String()+`"}}`)
	// The second deal starts where every deal must — a deal cannot be created
	// won — and is then moved, so the two differ in exactly the filtered field.
	outOfStage := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"deal","fields":{"name":"Narrowed out","pipeline_id":"`+
			pipeline.String()+`","stage_id":"`+open.String()+`"}}`)
	if _, err := registry.Invoke(ctx, "advance_deal", json.RawMessage(
		`{"deal_id":"`+outOfStage.String()+`","to_stage_id":"`+won.String()+`","won_without_contract_reason":"imported"}`)); err != nil {
		t.Fatalf("moving the second deal out of the filtered stage: %v", err)
	}

	out, err := registry.Invoke(ctx, "list_records", json.RawMessage(
		`{"record_type":"deal","filters":{"stage_id":"`+open.String()+`"}}`))
	if err != nil {
		t.Fatalf("listing deals in one stage: %v", err)
	}

	if !strings.Contains(string(out), inStage.String()) {
		t.Errorf("the deal in the filtered stage is missing from the answer:\n%s", out)
	}
	if strings.Contains(string(out), outOfStage.String()) {
		t.Errorf("a deal in another stage came back from a stage-filtered list — the filter reached "+
			"no predicate, so the answer is every row wearing the shape of a narrowed one:\n%s", out)
	}
}

// The same proof for the other operand kind this surface binds: an owner-scoped
// list. `owner_id` is the filter every record type carries, so a break in the
// reference binding would widen four enumerations at once.
func TestListRecordsNarrowsByOwner(t *testing.T) {
	e := Setup(t)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)

	mine := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"contact","fields":{"linkedin_url":"https://www.linkedin.com/in/fixture-a3997ea57d33b00e","full_name":"Owned By Rep One","owner_id":"`+e.Rep1.String()+`"}}`)
	theirs := createThroughTheToolSurface(ctx, t, registry,
		`{"record_type":"contact","fields":{"linkedin_url":"https://www.linkedin.com/in/fixture-ef7aadd5f7c1a1d2","full_name":"Owned By Rep Two","owner_id":"`+e.Rep2.String()+`"}}`)

	out, err := registry.Invoke(ctx, "list_records", json.RawMessage(
		`{"record_type":"contact","filters":{"owner_id":"`+e.Rep1.String()+`"}}`))
	if err != nil {
		t.Fatalf("listing contacts by owner: %v", err)
	}

	if !strings.Contains(string(out), mine.String()) {
		t.Errorf("the contact owned by the filtered owner is missing from the answer:\n%s", out)
	}
	if strings.Contains(string(out), theirs.String()) {
		t.Errorf("a contact owned by someone else came back from an owner-filtered list:\n%s", out)
	}
}

// An offset page through the tool is the HTTP list's page at that offset: the same rows in the same order.
func TestListRecordsAtAnOffsetAnswersTheRowsTheHTTPListAnswersThere(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Tool Offset", "admin@tooloffset.test", "Admin")
	for _, name := range []string{"Offset Alpha", "Offset Beta", "Offset Gamma", "Offset Delta"} {
		body := map[string]any{"display_name": name, "source": "manual"}
		if status := e.Call(t, "POST", "/v1/companies", body, nil, nil); status != http.StatusCreated {
			t.Fatalf("creating company %q = %d", name, status)
		}
	}
	var web offsetPage
	if status := e.Call(t, "GET", "/v1/companies?offset=1&limit=2", nil, nil, &web); status != http.StatusOK {
		t.Fatalf("GET /v1/companies?offset=1 = %d", status)
	}
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	ctx := apptest.AdminContext(t, e, AdminPerms)

	out, err := registry.Invoke(ctx, "list_records", json.RawMessage(`{"record_type":"company","offset":1,"limit":2}`))
	if err != nil {
		t.Fatalf("listing companies at an offset: %v", err)
	}
	var answer struct {
		Data struct {
			Records []struct {
				ID string `json:"id"`
			} `json:"records"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatalf("reading the list_records answer: %v", err)
	}
	tool := answer.Data
	toolRows, webRows := make([]string, 0, len(tool.Records)), make([]string, 0, len(web.Data))
	for _, record := range tool.Records {
		toolRows = append(toolRows, record.ID)
	}
	for _, row := range web.Data {
		webRows = append(webRows, row.ID)
	}
	if len(webRows) != 2 || !slices.Equal(toolRows, webRows) {
		t.Errorf("list_records at offset 1 answered %v, and the HTTP list answered %v", toolRows, webRows)
	}
	if tool.NextCursor == "" || web.Page.NextCursor == nil {
		t.Errorf("an offset page with rows after it carries next_cursor %q (tool) and %v (HTTP), want both", tool.NextCursor, web.Page.NextCursor)
	}

	_, err = registry.Invoke(ctx, "list_records", json.RawMessage(`{"record_type":"partner","offset":1}`))
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 ||
		fault.Fields[0].Field != "offset" || fault.Fields[0].Code != "offset_unsupported" {
		t.Errorf("a partner list at an offset answered %v, want 422 field=offset code=offset_unsupported", err)
	}
}

// Every type outside compose.OffsetListTypes is refused an offset before a
// module is asked, and every type inside it opens its list at one.
func TestSearchRefusesAnOffsetOnEveryOtherType(t *testing.T) {
	for _, et := range datasource.EntityTypes() {
		if slices.Contains(compose.OffsetListTypes(), et) {
			continue
		}
		_, err := (&compose.Provider{}).Search(t.Context(), datasource.SearchQuery{EntityTypes: []datasource.EntityType{et}, Offset: new(1)})
		if _, ok := errors.AsType[*datasource.OffsetUnsupportedError](err); !ok {
			t.Errorf("a %s list at an offset answered %v, want the offset refusal", et, err)
		}
	}
}

func TestEveryOffsetListTypePagesByOffset(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Offset customer", &e.Rep1)
	seed := map[datasource.EntityType]func(i int){
		datasource.EntityContact: func(i int) { e.SeedContact(t, fmt.Sprintf("Offset contact %d", i), &e.Rep1) },
		datasource.EntityCompany: func(i int) { e.SeedCompany(t, fmt.Sprintf("Offset company %d", i), &e.Rep1) },
		datasource.EntityLead: func(i int) {
			name, email := fmt.Sprintf("Offset lead %d", i), fmt.Sprintf("offset%d@lead.test", i)
			if _, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
				FullName: &name, Email: &email, Source: provenance.RecordSourceManual,
			}); err != nil {
				t.Fatal(err)
			}
		},
		datasource.EntityDeal:    func(i int) { e.SeedDeal(t, fmt.Sprintf("Offset deal %d", i), pipeline, open, &e.Rep1) },
		datasource.EntityProject: func(i int) { seedProject(e.Admin(), t, e, fmt.Sprintf("Offset project %d", i), company, &e.Rep1) },
	}
	provider := compose.NewProvider(e.Pool)
	for _, et := range compose.OffsetListTypes() {
		create, known := seed[et]
		if !known {
			t.Errorf("%s pages by offset and this test cannot seed it: add its seeder", et)
			continue
		}
		for i := range 3 {
			create(i)
		}
		read := func(offset *int, limit int) []ids.UUID {
			t.Helper()
			res, err := provider.Search(e.Admin(), datasource.SearchQuery{EntityTypes: []datasource.EntityType{et}, Offset: offset, Limit: limit})
			if err != nil {
				t.Fatalf("listing %s at offset %v: %v", et, offset, err)
			}
			out := make([]ids.UUID, 0, len(res.Records))
			for _, record := range res.Records {
				out = append(out, record.Ref.ID)
			}
			return out
		}
		whole := read(nil, 50)
		if got := read(new(1), 1); len(whole) < 3 || !slices.Equal(got, whole[1:2]) {
			t.Errorf("%s at offset 1 read %v, want the second row of %v", et, got, whole)
		}
	}
}
