// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func reportAct(t *testing.T, d *twoDoors, action string, args map[string]any) string {
	t.Helper()
	args["action"] = action
	return string(d.tool(t, "change_reports", args))
}

func firstReportMetric(t *testing.T, d *twoDoors) string {
	t.Helper()
	var catalog struct {
		Metrics []struct {
			ID string `json:"id"`
		} `json:"metrics"`
	}
	if status := d.e.Call(t, "GET", "/v1/analytics/metrics", nil, nil, &catalog); status != http.StatusOK || len(catalog.Metrics) == 0 {
		t.Fatalf("the metric catalog → %d with %d metrics", status, len(catalog.Metrics))
	}
	return catalog.Metrics[0].ID
}

// A saved report, created, changed and archived through the agent. An open draft
// stays the work of whoever is writing it.
func TestAnAgentWorksASavedReport(t *testing.T) {
	d := newTwoDoors(t, "agent-report", "read", "write")
	selection := AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{firstReportMetric(t, d)}}
	created := reportAct(t, d, "create_report", map[string]any{"request_body": AnyMap{
		"name": "Agent's report", "audience": "private", "selection": selection,
	}})
	report := decodeVersioned(t, created)
	report = decodeVersioned(t, reportAct(t, d, "update_report", map[string]any{
		"id": report.ID, "if_version": report.Version, "request_body": AnyMap{
			"name": "Agent's changed report", "audience": "private", "selection": selection,
		},
	}))
	var revisions struct {
		Data []struct {
			Revision int64 `json:"revision"`
		} `json:"data"`
	}
	raw := d.tool(t, "read_reports", map[string]any{"kind": "report_revisions", "id": report.ID})
	if err := json.Unmarshal(raw, &revisions); err != nil || len(revisions.Data) != 2 {
		t.Errorf("the changed report's revisions answered %s (%v), want two", raw, err)
	}
	reportAct(t, d, "archive_report", map[string]any{"id": report.ID})
	var name string
	var archived bool
	if err := d.e.Owner.QueryRow(t.Context(),
		`SELECT name, archived_at IS NOT NULL FROM report_definition WHERE id = $1`, report.ID).Scan(&name, &archived); err != nil {
		t.Fatalf("reading the report: %v", err)
	}
	if name != "Agent's changed report" || !archived {
		t.Errorf("the report reads name=%q archived=%v, want the agent's name and archived", name, archived)
	}
}

// An initiative goes from draft to live through the agent.
func TestAnAgentDefinesAnInitiative(t *testing.T) {
	d := newTwoDoors(t, "agent-initiative", "read", "write")
	id := answerID(t, []byte(reportAct(t, d, "create_initiative", map[string]any{"request_body": AnyMap{
		"name": "Win back the fair", "objective_metric": firstReportMetric(t, d),
		"deadline": clocktest.Now(t).AddDate(0, 3, 0).Format("2006-01-02"),
	}})))
	initiatives := d.tool(t, "read_reports", map[string]any{"kind": "sales_initiative", "id": id})
	if !strings.Contains(string(initiatives), "Win back the fair") {
		t.Errorf("reading the initiative answered %s", initiatives)
	}
}

// What the forecast page writes: a call, and a link for a colleague.
func TestAnAgentCallsTheForecastAndSharesIt(t *testing.T) {
	d := newTwoDoors(t, "agent-forecast-writes", "read", "write")
	reportAct(t, d, "record_forecast_call", map[string]any{"request_body": AnyMap{"amount_minor": 1200000, "currency": "EUR"}})
	share := answerID(t, []byte(reportAct(t, d, "create_forecast_share", map[string]any{"request_body": AnyMap{
		"kind": "live", "target": "forecast",
	}})))
	reportAct(t, d, "revoke_forecast_share", map[string]any{"id": share})
}

// The coverage read answers the agent as it answers its user.
func TestAnAgentReadsCompanyCoverage(t *testing.T) {
	d := newTwoDoors(t, "agent-coverage", "read")
	d.tool(t, "read_reports", map[string]any{"kind": "company_coverage"})
}

// A project's health reading says an agent recorded it.
func TestAnAgentsProjectHealthReadingNamesTheAgent(t *testing.T) {
	d := newTwoDoors(t, "agent-project-health", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Health Works"})
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{"name": "Rollout", "company_id": company, "source": "manual"})
	d.tool(t, "change_state", map[string]any{
		"action": "record_project_health", "id": project,
		"request_body": AnyMap{"state": "at_risk", "note": "The pilot slipped a month."},
	})
	var source, capturedBy string
	if err := d.e.Owner.QueryRow(t.Context(),
		`SELECT source, captured_by FROM project_health_assessment WHERE project_id = $1`, project).Scan(&source, &capturedBy); err != nil {
		t.Fatalf("reading the reading: %v", err)
	}
	if source != "agent" || !isAgentStamp(capturedBy) {
		t.Errorf("source=%q captured_by=%q, want the agent", source, capturedBy)
	}
}

// A saved view and a list's visit mark are the user's own, so the agent's writes
// land on the user's rows.
func TestAnAgentKeepsTheUsersViewsAndVisitMark(t *testing.T) {
	d := newTwoDoorsWith(t, "agent-views", []compose.Option{compose.WithListsEnabled(true)}, "read", "write")
	view := answerID(t, d.tool(t, "change_settings", map[string]any{"action": "create_saved_view", "request_body": AnyMap{
		"resource": "contacts", "name": "My contacts", "query": AnyMap{"columns": []any{"full_name"}},
	}}))
	if status := d.e.Call(t, "GET", "/v1/views/"+view, nil, nil, nil); status != http.StatusOK {
		t.Errorf("the user reading the view their agent saved → %d", status)
	}
	d.tool(t, "change_settings", map[string]any{"action": "archive_saved_view", "id": view})

	list := createdRecord(t, d.e, "/v1/lists", AnyMap{"name": "Fair leads", "entity_type": "contact", "list_type": "static"})
	d.tool(t, "change_attention", map[string]any{"action": "visit_list", "id": list})
	var visitor string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT user_id::text FROM list_visit WHERE list_id = $1`, list).Scan(&visitor); err != nil {
		t.Fatalf("the agent's visit left no mark on the user's row: %v", err)
	}
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := d.e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK || visitor != me.User.ID {
		t.Errorf("the visit marks %q, and the user is %q (%d)", visitor, me.User.ID, status)
	}
}
