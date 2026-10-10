// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Five agent tools whose answer a route gives, by the same engine or through the
// registry. Each test seeds through the product's own writers. It asks the
// route as a passport and the tool over MCP with the same input, and compares
// the records the two answers carry.

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// twoDoors is one workspace with a signed-in admin and one passport. REST calls
// present the passport as a bearer, and the MCP client calls tools with it.
type twoDoors struct {
	e      *apptest.AppEnv
	bearer map[string]string
	mcp    *apptest.MCPClient
}

func newTwoDoors(t *testing.T, slug string, scopes ...string) *twoDoors {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"),
			compose.WithBlobstore(blobstore.NewMemory()),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Two Doors", slug+"@fable.test", "Admin")
	bearer, _ := passportWithID(t, e, slug+" agent", scopes...)
	return &twoDoors{e: e, bearer: bearer, mcp: apptest.NewMCPClient(e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))}
}

// rest answers a route's body as the passport, failing on anything but 200.
//
//craft:ignore naked-any the request body is whichever JSON shape the route under test takes
func (d *twoDoors) rest(t *testing.T, method, path string, body any) json.RawMessage {
	t.Helper()
	var out json.RawMessage
	if status := d.e.Call(t, method, path, body, d.bearer, &out); status != http.StatusOK {
		t.Fatalf("%s %s as a passport → %d %s", method, path, status, out)
	}
	return out
}

func (d *twoDoors) tool(t *testing.T, name string, args map[string]any) json.RawMessage {
	t.Helper()
	return d.mcp.CallOK(t, name, args).Envelope(t).Data
}

// sameRecords decodes both answers into one shape holding only the members
// the two doors share, and fails when they differ.
func sameRecords[T any](t *testing.T, what string, rest, tool json.RawMessage) T {
	t.Helper()
	var fromREST, fromTool T
	if err := json.Unmarshal(rest, &fromREST); err != nil {
		t.Fatalf("%s: the route's answer does not decode: %v\n%s", what, err, rest)
	}
	if err := json.Unmarshal(tool, &fromTool); err != nil {
		t.Fatalf("%s: the tool's answer does not decode: %v\n%s", what, err, tool)
	}
	if !reflect.DeepEqual(fromREST, fromTool) {
		t.Errorf("%s: the route answered\n%+v\nthe tool answered\n%+v", what, fromREST, fromTool)
	}
	return fromREST
}

type sharedCoverage struct {
	DealID       string `json:"deal_id"`
	Stakeholders []struct {
		ContactID   string `json:"contact_id"`
		ContactName string `json:"contact_name"`
		Role        string `json:"role"`
		Engaged     bool   `json:"engaged"`
	} `json:"stakeholders"`
	OurSide []struct {
		UserID         string `json:"user_id"`
		StrengthBucket string `json:"strength_bucket"`
	} `json:"our_side"`
	Risks []struct {
		Kind       string   `json:"kind"`
		ContactIDs []string `json:"contact_ids"`
	} `json:"risks"`
}

type sharedNetwork struct {
	ContactID  string `json:"contact_id"`
	Colleagues []struct {
		UserID          string `json:"user_id"`
		DisplayName     string `json:"display_name"`
		StrengthBucket  string `json:"strength_bucket"`
		Interactions90d int    `json:"interactions_90d"`
	} `json:"colleagues"`
}

// The deal's seats and the colleague who called a contact come back alike from
// GET /deals/{id}/coverage and company_coverage, and from
// GET /contacts/{id}/network and who_knows.
func TestCoverageAndNetworkRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "coverage-doors", "read")
	company, deal := dealAtAnAccount(t, d.e, "Hollow Ridge", "Ridge renewal")
	champion := contactAt(t, d.e, company, "Ines Champion", "")
	buyer := contactAt(t, d.e, company, "Bo Buyer", "")
	stakeholder(t, d.e, deal, champion, "champion")
	stakeholder(t, d.e, deal, buyer, "economic_buyer")
	callAndFold(t, d.e, champion)

	coverage := sameRecords[sharedCoverage](t, "coverage",
		d.rest(t, "GET", "/v1/deals/"+deal+"/coverage", nil),
		d.tool(t, "company_coverage", map[string]any{"deal_id": deal}))
	if len(coverage.Stakeholders) != 2 {
		t.Errorf("the coverage answer seats %d stakeholders, want the 2 seeded", len(coverage.Stakeholders))
	}

	network := sameRecords[sharedNetwork](t, "network",
		d.rest(t, "GET", "/v1/contacts/"+champion+"/network", nil),
		d.tool(t, "who_knows", map[string]any{"contact_id": champion}))
	if len(network.Colleagues) != 1 {
		t.Errorf("the network answer names %d colleagues, want the admin who called", len(network.Colleagues))
	}
}

// callAndFold logs an outbound call to the contact as the admin, then folds the
// interaction projection the way its event consumer does.
func callAndFold(t *testing.T, e *apptest.AppEnv, contact string) {
	t.Helper()
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "call", "source": "manual", "direction": "outbound", "subject": "Renewal call",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("logging the call → %d", status)
	}
	contactID, err := ids.Parse(contact)
	if err != nil {
		t.Fatal(err)
	}
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForContact(context.Background(), tx, contactID)
	}); err != nil {
		t.Fatalf("folding the interaction projection: %v", err)
	}
}

type sharedPipelines struct {
	Pipelines []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		IsDefault bool   `json:"is_default"`
		Position  int    `json:"position"`
		Stages    []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Semantic       string `json:"semantic"`
			WinProbability int    `json:"win_probability"`
			Position       int    `json:"position"`
		} `json:"stages"`
	} `json:"pipelines"`
}

// GET /pipelines carries its list under `data`, the tool under `pipelines`;
// the pipelines and their stages are the same.
func TestPipelinesRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "pipeline-doors", "read")
	var page struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(d.rest(t, "GET", "/v1/pipelines", nil), &page); err != nil {
		t.Fatalf("the pipeline list does not decode: %v", err)
	}
	rest := json.RawMessage(`{"pipelines":` + string(page.Data) + `}`)
	got := sameRecords[sharedPipelines](t, "pipelines", rest, d.tool(t, "list_pipelines", map[string]any{}))
	if len(got.Pipelines) == 0 || len(got.Pipelines[0].Stages) == 0 {
		t.Errorf("the seeded pipeline came back without its stages: %+v", got)
	}
}

// GET /query-vocabulary runs describe_query_vocabulary itself, so the two bodies
// are one JSON document. A human session reads it too.
func TestQueryVocabularyRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "vocabulary-doors", "read")
	rest := d.rest(t, "GET", "/v1/query-vocabulary", nil)
	if compacted(t, rest) != compacted(t, d.tool(t, "describe_query_vocabulary", map[string]any{})) {
		t.Errorf("GET /v1/query-vocabulary and describe_query_vocabulary answered different documents")
	}
	if !strings.Contains(string(rest), `"targets"`) {
		t.Errorf("the vocabulary names no targets: %s", rest)
	}
	if status := d.e.Call(t, "GET", "/v1/query-vocabulary", nil, nil, nil); status != http.StatusOK {
		t.Errorf("a human's GET /v1/query-vocabulary → %d, want 200", status)
	}
}

// A report citing a saved run renders the same figures on
// POST /analytics/reports/render and compose_analytics_report.
func TestReportRenderRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "report-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	for i := range 6 {
		if status := d.e.Call(t, "POST", "/v1/deals", AnyMap{
			"name": "Open deal " + string(rune('A'+i)), "pipeline_id": stages.PipelineID,
			"stage_id": stages.Open, "source": "manual",
		}, nil, nil); status != http.StatusCreated {
			t.Fatalf("seeding open deal %d → %d", i, status)
		}
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(d.rest(t, "POST", "/v1/analytics/query", AnyMap{
		"entity":   "open-deals-per-company",
		"measures": []AnyMap{{"fn": "count", "as": "dealcount"}}, "save": true,
	}), &run); err != nil || run.RunID == "" {
		t.Fatalf("the analytics query saved no run (%v)", err)
	}
	doc := map[string]any{"blocks": []any{
		map[string]any{"kind": "title", "text": "Open pipeline"},
		map[string]any{"kind": "stat_strip", "cells": []any{map[string]any{"run_id": run.RunID, "column": "dealcount"}}},
	}}
	rendered := sameRecords[map[string]any](t, "report",
		d.rest(t, "POST", "/v1/analytics/reports/render", doc), d.tool(t, "compose_analytics_report", doc))
	if !strings.Contains(compactedAny(t, rendered), `"value":6`) {
		t.Errorf("the stat strip does not show the 6 open deals: %v", rendered)
	}
}

//craft:ignore naked-any the value is whichever decoded JSON the caller compared
func compactedAny(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
