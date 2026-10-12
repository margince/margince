// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The agent tools that search, query and resolve, each served on its own route
// by the registry. The lane wires no embedder, so ranking runs on the lexical
// lane alone, the same on both doors. Every test seeds through the product's
// writers and holds the route's body and the tool's payload to one document.

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

// GET /context-search answers search_context's ranked records, and
// POST /workspace-queries answers query_workspace's rows, for one input.
func TestSearchAndQueryRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "search-routes", "read")
	turbine := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Annegret Turbinenbau"})
	createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Bernd Unrelated"})

	ranked := d.rest(t, "GET", "/v1/context-search?query=Turbinenbau&record_types=contact&limit=5", nil)
	if compacted(t, ranked) != compacted(t, d.tool(t, "search_context", map[string]any{
		"query": "Turbinenbau", "record_types": []string{"contact"}, "limit": 5,
	})) {
		t.Errorf("GET /v1/context-search and search_context answered different documents:\n%s", ranked)
	}
	if got := recordIDs(t, ranked, "hits"); !slices.Equal(got, []string{turbine}) {
		t.Errorf("the context search found %v, want the one Turbinenbau contact", got)
	}

	plan := map[string]any{"plan": map[string]any{
		"version": "v1", "target": "contact",
		"where": []any{map[string]any{"field": "full_name", "op": "eq", "value": "Annegret Turbinenbau"}},
	}}
	rows := d.rest(t, "POST", "/v1/workspace-queries", plan)
	if compacted(t, rows) != compacted(t, d.tool(t, "query_workspace", plan)) {
		t.Errorf("POST /v1/workspace-queries and query_workspace answered different documents:\n%s", rows)
	}
	if got := recordIDs(t, rows, "rows"); !slices.Equal(got, []string{turbine}) {
		t.Errorf("the plan matched %v, want the one contact it names", got)
	}

	var refused json.RawMessage
	if status := d.e.Call(t, "POST", "/v1/workspace-queries", AnyMap{"plan": plan["plan"], "limits": 3},
		d.bearer, &refused); status != http.StatusUnprocessableEntity {
		t.Errorf("a body member the tool does not take → %d %s, want 422 as over MCP", status, refused)
	}
}

// POST /entity-resolutions resolves the same payload to the same record as
// resolve_entities, and echoes the caller's label.
func TestEntityResolutionRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "resolve-routes", "read")
	anna := createdRecord(t, d.e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Anna Acme", "emails": []AnyMap{{"email": "anna@acme.example", "is_primary": true}},
	})
	payload := map[string]any{"candidates": []any{
		map[string]any{"kind": "contact", "ref": "card", "emails": []string{"anna@acme.example"}},
	}}
	resolved := d.rest(t, "POST", "/v1/entity-resolutions", payload)
	if compacted(t, resolved) != compacted(t, d.tool(t, "resolve_entities", payload)) {
		t.Errorf("POST /v1/entity-resolutions and resolve_entities answered different documents:\n%s", resolved)
	}
	var answer struct {
		Candidates []struct {
			Ref     string `json:"ref"`
			Matches []struct {
				Record struct {
					ID string `json:"id"`
				} `json:"record"`
			} `json:"matches"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(resolved, &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Candidates) != 1 || answer.Candidates[0].Ref != "card" ||
		len(answer.Candidates[0].Matches) != 1 || answer.Candidates[0].Matches[0].Record.ID != anna {
		t.Errorf("the payload resolved to %+v, want Anna's record under the label card", answer.Candidates)
	}
}

// POST /analytics/runs/{run_id}/evidence-search judges a saved run's records
// as search_report_evidence does. The path names the run, and a run_id in the
// body cannot point the search at another one.
func TestReportEvidenceRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "evidence-routes", "read")
	for i := range 6 {
		body := "Walked through the onboarding plan"
		if i < 3 {
			body = "The buyer pushed back on pricing again"
		}
		if status := d.e.Call(t, "POST", "/v1/activities", AnyMap{
			"kind": "call", "source": "manual", "direction": "outbound", "subject": "Weekly check-in", "body": body,
		}, nil, nil); status != http.StatusCreated {
			t.Fatalf("logging call %d → %d", i, status)
		}
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(d.rest(t, "POST", "/v1/analytics/query", AnyMap{
		"entity": "activities-by-kind", "group_by": []string{"kind"},
		"measures": []AnyMap{{"fn": "count", "as": "activities"}}, "save": true,
	}), &run); err != nil || run.RunID == "" {
		t.Fatalf("the analytics query saved no run (%v)", err)
	}

	evidence := d.rest(t, "POST", "/v1/analytics/runs/"+run.RunID+"/evidence-search",
		AnyMap{"query": "pricing", "run_id": "00000000-0000-7000-8000-000000000000"})
	if compacted(t, evidence) != compacted(t, d.tool(t, "search_report_evidence",
		map[string]any{"run_id": run.RunID, "query": "pricing"})) {
		t.Errorf("the evidence route and search_report_evidence answered different documents:\n%s", evidence)
	}
	var tally struct {
		Tally struct {
			Matched   int `json:"matched"`
			Unmatched int `json:"unmatched"`
		} `json:"tally"`
	}
	if err := json.Unmarshal(evidence, &tally); err != nil {
		t.Fatal(err)
	}
	if tally.Tally.Matched+tally.Tally.Unmatched == 0 {
		t.Errorf("the search judged none of the run's six calls: %s", evidence)
	}
}

// recordIDs reads the record ids out of a list of hits or rows.
func recordIDs(t *testing.T, answer json.RawMessage, list string) []string {
	t.Helper()
	var members map[string]json.RawMessage
	var items []struct {
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
	}
	if err := json.Unmarshal(answer, &members); err != nil || json.Unmarshal(members[list], &items) != nil {
		t.Fatalf("the answer's %s do not decode:\n%s", list, answer)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Record.ID)
	}
	return ids
}
