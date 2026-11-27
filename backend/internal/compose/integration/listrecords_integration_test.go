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
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
