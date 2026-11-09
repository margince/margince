// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// versioned reads the id, version and revision a report write answered.
type versioned struct {
	ID       string `json:"id"`
	Version  int64  `json:"version"`
	Revision int64  `json:"revision"`
}

func decodeVersioned(t *testing.T, raw string) versioned {
	t.Helper()
	var got versioned
	if err := json.Unmarshal([]byte(raw), &got); err != nil || got.ID == "" {
		t.Fatalf("the answer %s names no id (%v)", raw, err)
	}
	return got
}

// auditRowsBy is the number of audit rows of one kind one sort of actor wrote.
func auditRowsBy(t *testing.T, d *twoDoors, actorType, entityType string) int {
	t.Helper()
	var n int
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT count(*) FROM audit_log WHERE actor_type = $1 AND entity_type = $2`, actorType, entityType).Scan(&n); err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	return n
}

// A saved report goes through every state an editor takes it through, and each
// write the agent makes reads as the agent's.
func TestAnAgentTakesAReportThroughItsLife(t *testing.T) {
	d := newTwoDoors(t, "agent-report-life", "read", "write")
	selection := AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{firstReportMetric(t, d)}}
	input := func(name string) AnyMap { return AnyMap{"name": name, "audience": "private", "selection": selection} }
	created := reportAct(t, d, "create_report", map[string]any{"request_body": input("First name")})
	if strings.Contains(created, `"blocks":null`) {
		t.Errorf("a report with no charts answered blocks as null, where the contract requires a list: %s", created)
	}
	report := decodeVersioned(t, created)

	report = decodeVersioned(t, reportAct(t, d, "update_report", map[string]any{
		"id": report.ID, "if_version": report.Version, "request_body": input("Second name"),
	}))
	if report.Revision != 2 {
		t.Errorf("the agent's change left revision %d, want 2", report.Revision)
	}
	report = decodeVersioned(t, reportAct(t, d, "restore_revision", map[string]any{
		"id": report.ID, "revision": "1", "if_version": report.Version,
	}))

	if _, err := d.e.Owner.Exec(t.Context(), `
		INSERT INTO retention_policy (object_type, retain_days, action, enabled) VALUES ('report_edition', 730, 'erase', true)`); err != nil {
		t.Fatalf("seeding the retention an edition needs: %v", err)
	}
	schedule := decodeVersioned(t, reportAct(t, d, "create_schedule", map[string]any{"id": report.ID, "request_body": AnyMap{
		"report_revision": report.Revision, "frequency": "monthly", "day": 28, "local_time": "09:00", "enabled": true,
	}}))
	reportAct(t, d, "update_schedule", map[string]any{"id": schedule.ID, "if_version": schedule.Version, "request_body": AnyMap{
		"report_revision": report.Revision, "frequency": "monthly", "day": 28, "local_time": "10:30", "enabled": true,
	}})
	var frozen struct {
		ReportID string `json:"report_id"`
		Status   string `json:"status"`
	}
	if raw := reportAct(t, d, "freeze_edition", map[string]any{"id": report.ID}); json.Unmarshal([]byte(raw), &frozen) != nil ||
		frozen.ReportID != report.ID || frozen.Status != "pending" {
		t.Errorf("freeze_edition answered %s, want a pending run of this report", raw)
	}

	report = decodeVersioned(t, reportAct(t, d, "draft", map[string]any{
		"report_id": report.ID, "selection": selection,
	}))
	report = decodeVersioned(t, reportAct(t, d, "publish_draft", map[string]any{"id": report.ID, "if_version": report.Version}))
	published := report.Revision
	report = decodeVersioned(t, reportAct(t, d, "draft", map[string]any{
		"report_id": report.ID, "selection": selection,
	}))
	reportAct(t, d, "discard_draft", map[string]any{"id": report.ID, "if_version": report.Version})

	var asUser struct {
		Revision      int64  `json:"revision"`
		DraftRevision *int64 `json:"draft_revision"`
	}
	if status := d.e.Call(t, "GET", "/v1/analytics/reports/"+report.ID, nil, nil, &asUser); status != http.StatusOK {
		t.Fatalf("the user's read → %d", status)
	}
	if asUser.Revision != published || asUser.DraftRevision != nil {
		t.Errorf("the user reads revision %d with draft %v, want the published %d and no draft", asUser.Revision, asUser.DraftRevision, published)
	}
	if rows := auditRowsBy(t, d, "human", "report_definition"); rows != 0 {
		t.Errorf("%d report writes read as the user's", rows)
	}
	if rows := auditRowsBy(t, d, "agent", "report_definition"); rows == 0 {
		t.Error("no report write names the agent")
	}
}

// A target and the reporting framework, corrected through the agent.
func TestAnAgentCorrectsATargetAndTheFramework(t *testing.T) {
	d := newTwoDoors(t, "agent-report-target", "read", "write")
	start := clocktest.Now(t).Format("2006-01") + "-01"
	input := func(value int, reason string) AnyMap {
		return AnyMap{
			"metric": "meetings_held", "scope": AnyMap{"kind": "workspace"}, "period_kind": "month",
			"period_start": start, "value": value, "reason": reason,
		}
	}
	target := decodeVersioned(t, reportAct(t, d, "create_target", map[string]any{"request_body": input(40, "The plan for the month.")}))
	corrected := decodeVersioned(t, reportAct(t, d, "update_target", map[string]any{
		"id": target.ID, "if_version": target.Version, "request_body": input(45, "More capacity."),
	}))
	if corrected.Revision != 2 {
		t.Errorf("the corrected target is revision %d, want 2", corrected.Revision)
	}

	var framework struct {
		Version    int64          `json:"version"`
		Revision   int64          `json:"revision"`
		Definition map[string]any `json:"definition"`
	}
	if status := d.e.Call(t, "GET", "/v1/analytics/framework", nil, nil, &framework); status != http.StatusOK {
		t.Fatalf("the user's framework read → %d", status)
	}
	framework.Definition["template"], framework.Definition["reason"] = "sdr", "Use agreed SDR outcomes"
	revised := reportAct(t, d, "publish_framework", map[string]any{"if_version": framework.Version, "request_body": framework.Definition})
	var after struct {
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal([]byte(revised), &after); err != nil || after.Revision != framework.Revision+1 {
		t.Errorf("publishing the framework answered %s (%v), want revision %d", revised, err, framework.Revision+1)
	}
	var targetRows, frameworkRows int
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE entity_id::text = $1), count(*) FILTER (WHERE entity_type LIKE '%framework%')
		  FROM audit_log WHERE actor_type = 'agent' AND passport_id IS NOT NULL AND on_behalf_of IS NOT NULL`,
		target.ID).Scan(&targetRows, &frameworkRows); err != nil {
		t.Fatalf("reading the audit rows: %v", err)
	}
	if targetRows != 2 || frameworkRows != 1 {
		t.Errorf("the audit names the agent on %d target writes and %d framework writes, want 2 and 1", targetRows, frameworkRows)
	}
}

// An initiative is changed and launched, with the user's leads picked by hand.
func TestAnAgentChangesAndLaunchesAnInitiative(t *testing.T) {
	d := newTwoDoors(t, "agent-report-initiative", "read", "write")
	metric := firstReportMetric(t, d)
	deadline := clocktest.Now(t).AddDate(0, 3, 0).Format(time.DateOnly)
	created := decodeVersioned(t, reportAct(t, d, "create_initiative", map[string]any{"request_body": AnyMap{
		"name": "Win back the fair", "objective_metric": metric, "deadline": deadline,
	}}))
	changed := decodeVersioned(t, reportAct(t, d, "update_initiative", map[string]any{
		"id": created.ID, "if_version": created.Version, "request_body": AnyMap{
			"name": "Win back the spring fair", "objective_metric": metric, "deadline": deadline,
		},
	}))
	lead := createdRecord(t, d.e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Fair Lead", "email": "lead@fair.test"})
	reportAct(t, d, "replace_initiative_members", map[string]any{"id": changed.ID, "request_body": AnyMap{
		"picked": []AnyMap{{"record_type": "lead", "record_id": lead}}, "excluded": []AnyMap{},
	}})
	current := decodeVersioned(t, string(d.tool(t, "read_reports", map[string]any{"kind": "sales_initiative", "id": changed.ID})))
	launched := reportAct(t, d, "launch_initiative", map[string]any{"id": changed.ID, "if_version": current.Version})
	var state struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(launched), &state); err != nil || state.Status != "live" {
		t.Errorf("launching answered %s (%v), want live", launched, err)
	}
	var capturedBy string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT captured_by FROM sales_initiative_member WHERE initiative_id = $1`, changed.ID).Scan(&capturedBy); err != nil {
		t.Fatalf("reading the picked member: %v", err)
	}
	if !isAgentStamp(capturedBy) {
		t.Errorf("the picked member names %q, want the agent", capturedBy)
	}
}

// A decision on a real deal and its later outcome, as the review page records them.
func TestAnAgentRecordsADecisionAndItsOutcome(t *testing.T) {
	d := newTwoDoors(t, "agent-report-decision", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Decision Works"})
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := createdID(t, d.e, "/v1/deals", AnyMap{
		"source": "manual", "name": "Decided deal", "company_id": company,
		"pipeline_id": stages.PipelineID, "stage_id": stages.Open, "owner_id": memberID(t, d, "agent-report-decision@fable.test"),
	})
	decision := answerID(t, []byte(reportAct(t, d, "record_decision", map[string]any{"request_body": AnyMap{
		"decision": "Offer the pilot at cost.", "deal_id": deal, "owner_id": memberID(t, d, "agent-report-decision@fable.test"),
		"review_on": clocktest.Now(t).AddDate(0, 1, 0).Format(time.DateOnly),
	}})))
	reportAct(t, d, "record_decision_outcome", map[string]any{"id": decision, "request_body": AnyMap{"result": "They took the pilot."}})
}
