// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

func settingsAct(t *testing.T, d *twoDoors, action string, args map[string]any) json.RawMessage {
	t.Helper()
	args["action"] = action
	return d.tool(t, "change_settings", args)
}

// actorOfLastAudit reads who wrote the latest audit row about one entity.
func actorOfLastAudit(t *testing.T, d *twoDoors, entityType, entityID string) string {
	t.Helper()
	var actorType string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT actor_type FROM audit_log WHERE entity_type = $1 AND entity_id = $2
		ORDER BY occurred_at DESC, id DESC LIMIT 1`, entityType, entityID).Scan(&actorType); err != nil {
		t.Fatalf("reading the audit row of %s %s: %v", entityType, entityID, err)
	}
	return actorType
}

// A pipeline an agent builds is a pipeline the app shows, and its audit names
// the agent. Which stage closes a deal stays with a user: relabelling a stage
// would lift the approval automatic work needs to move a deal to won.
func TestAnAgentSetsUpAPipelineButNeverWhatClosesADeal(t *testing.T) {
	d := newTwoDoors(t, "agent-pipeline", "read", "write")
	for name, call := range map[string]map[string]any{
		"a pipeline with a won stage": {"action": "create_pipeline", "request_body": AnyMap{"name": "Closes itself", "stages": []AnyMap{
			{"name": "Done", "position": 1, "semantic": "won"},
		}}},
		"a pipeline with a lost stage": {"action": "create_pipeline", "request_body": AnyMap{"name": "Loses itself", "stages": []AnyMap{
			{"name": "Gone", "position": 1, "semantic": "lost"},
		}}},
	} {
		if refusal := d.toolRefused(t, "change_settings", call); !strings.Contains(refusal, "an agent does not") {
			t.Errorf("%s answered %q", name, refusal)
		}
	}
	pipeline := answerID(t, settingsAct(t, d, "create_pipeline", map[string]any{"request_body": AnyMap{
		"name": "Partners", "stages": []AnyMap{{"name": "Talking", "position": 1, "semantic": "open"}},
	}}))
	for _, semantic := range []string{"won", "lost"} {
		refusal := d.toolRefused(t, "change_settings", map[string]any{"action": "create_stage", "request_body": AnyMap{
			"pipeline_id": pipeline, "name": "Closing " + semantic, "position": 7, "semantic": semantic,
		}})
		if !strings.Contains(refusal, "an agent does not") {
			t.Errorf("creating a %s stage answered %q", semantic, refusal)
		}
	}
	settingsAct(t, d, "create_stage", map[string]any{"request_body": AnyMap{"pipeline_id": pipeline, "name": "Reviewing", "position": 2, "semantic": "open"}})
	if status := d.e.Call(t, "POST", "/v1/stages", AnyMap{"pipeline_id": pipeline, "name": "Signed", "position": 3, "semantic": "won"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("the user adding a won stage → %d", status)
	}
	if got := actorOfLastAudit(t, d, "pipeline", pipeline); got != "agent" {
		t.Errorf("the pipeline's audit row names %q, want the agent", got)
	}
	var viaREST struct {
		Name   string `json:"name"`
		Stages []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Semantic string `json:"semantic"`
		} `json:"stages"`
	}
	if status := d.e.Call(t, "GET", "/v1/pipelines/"+pipeline, nil, nil, &viaREST); status != http.StatusOK || viaREST.Name != "Partners" {
		t.Fatalf("the user's own door reads the pipeline as %d %+v", status, viaREST)
	}
	stageNamed := func(name string) string {
		for _, stage := range viaREST.Stages {
			if stage.Name == name {
				return stage.ID
			}
		}
		t.Fatalf("no stage %s in %+v", name, viaREST.Stages)
		return ""
	}

	settingsAct(t, d, "update_stage", map[string]any{"id": stageNamed("Talking"), "request_body": AnyMap{"name": "In talks"}})
	if got := actorOfLastAudit(t, d, "stage", stageNamed("Talking")); got != "agent" {
		t.Errorf("the stage's audit row names %q, want the agent", got)
	}
	settingsAct(t, d, "update_stage", map[string]any{"id": stageNamed("Signed"), "request_body": AnyMap{"semantic": "won"}})

	refusal := d.toolRefused(t, "change_settings", map[string]any{
		"action": "update_stage", "id": stageNamed("Signed"), "request_body": AnyMap{"semantic": "open"},
	})
	if !strings.Contains(refusal, "an agent does not decide whether a stage counts") {
		t.Errorf("relabelling a won stage answered %q", refusal)
	}
	if status := d.e.Call(t, "PATCH", "/v1/stages/"+stageNamed("Signed"), AnyMap{"semantic": "open"}, d.bearer, nil); status != http.StatusForbidden {
		t.Errorf("the same relabel over the passport's REST door → %d, want 403", status)
	}
	if status := d.e.Call(t, "PATCH", "/v1/stages/"+stageNamed("Signed"), AnyMap{"semantic": "open"}, nil, nil); status != http.StatusOK {
		t.Errorf("the user relabelling their stage → %d, want 200", status)
	}
}

// A partner's terms open to an agent, and the margin tier stays a user's: it
// decides commission on every later deal. Clearing it is the same act.
func TestAnAgentSetsAPartnersTermsButNeverItsMarginTier(t *testing.T) {
	d := newTwoDoors(t, "agent-partner", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Channel Co"})
	settingsAct(t, d, "upsert_partner", map[string]any{"id": company, "request_body": AnyMap{"partner_kinds": []string{"referral"}}})
	var partner struct {
		PartnerKinds []string `json:"partner_kinds"`
		MarginTier   *string  `json:"margin_tier"`
		CapturedBy   string   `json:"captured_by"`
		Version      int64    `json:"version"`
	}
	if status := d.e.Call(t, "GET", "/v1/companies/"+company+"/partner", nil, nil, &partner); status != http.StatusOK {
		t.Fatalf("reading the partner → %d", status)
	}
	if len(partner.PartnerKinds) != 1 || partner.PartnerKinds[0] != "referral" || partner.MarginTier != nil {
		t.Errorf("the partner reads %+v", partner)
	}
	for name, body := range map[string]AnyMap{
		"setting":  {"partner_kinds": []string{"referral"}, "margin_tier": "tier1_15"},
		"clearing": {"partner_kinds": []string{"referral"}, "margin_tier": nil},
	} {
		refusal := d.toolRefused(t, "change_settings", map[string]any{
			"action": "upsert_partner", "id": company, "if_version": partner.Version, "request_body": body,
		})
		if !strings.Contains(refusal, "margin tier") {
			t.Errorf("%s the margin tier answered %q", name, refusal)
		}
	}
}

// A web form's secret is a way in, so an agent neither sets nor clears it, and
// the refusal comes before anything judges the secret. The rest is open.
func TestAnAgentChangesAWebFormButNeverItsSecret(t *testing.T) {
	d := newTwoDoors(t, "agent-webform", "read", "write")
	var form struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
		Detail  string `json:"detail"`
	}
	if status := d.e.Call(t, "POST", "/v1/web-forms", AnyMap{
		"name": "Contact us", "sender": "browser", "allowed_origins": []string{"https://site.example"},
	}, nil, &form); status != http.StatusCreated {
		t.Fatalf("creating the form → %d %s", status, form.Detail)
	}
	got := settingsAct(t, d, "update_web_form", map[string]any{"id": form.ID, "if_version": form.Version, "request_body": AnyMap{"name": "Talk to us"}})
	if !strings.Contains(string(got), "Talk to us") {
		t.Errorf("renaming the form answered %s", got)
	}
	for what, secret := range map[string]any{"setting": "a-secret-an-agent-chose-1", "clearing": nil} {
		refusal := d.toolRefused(t, "change_settings", map[string]any{
			"action": "update_web_form", "id": form.ID, "if_version": form.Version + 1, "request_body": AnyMap{"sender_secret": secret},
		})
		if !strings.Contains(refusal, "an agent does not set or clear a form's secret") {
			t.Errorf("%s the secret answered %q", what, refusal)
		}
	}
	if status := d.e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, AnyMap{"sender_secret": "a-secret-an-agent-chose-2"}, withIfMatch(d.bearer, form.Version+1), nil); status != http.StatusForbidden {
		t.Errorf("setting the secret over the passport's REST door → %d, want 403", status)
	}
}

// A former colleague is recorded for imported history. An agent never names the
// admin role on one, and its audit row names it.
func TestAnAgentRecordsAFormerColleagueButNeverAnAdmin(t *testing.T) {
	d := newTwoDoors(t, "agent-former", "read", "write")
	if status := d.e.Call(t, "PUT", "/v1/company", wellFormedCompany(), nil, nil); status != http.StatusOK {
		t.Fatalf("describing the company → %d", status)
	}
	refusal := d.toolRefused(t, "change_access", map[string]any{"action": "create_former_member", "request_body": AnyMap{
		"email": "left-admin@old.test", "display_name": "Left Admin", "role": "admin",
	}})
	if !strings.Contains(refusal, "an agent does not grant the admin role") {
		t.Errorf("a former admin answered %q", refusal)
	}
	id := answerID(t, d.tool(t, "change_access", map[string]any{"action": "create_former_member", "request_body": AnyMap{
		"email": "left-rep@old.test", "display_name": "Left Rep",
	}}))
	var actorType string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT actor_type FROM audit_log WHERE entity_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&actorType); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if actorType != "agent" {
		t.Errorf("the former member's audit row names %q, want the agent", actorType)
	}
}

// A custom field an agent adds is a column the user's own door reads, and its
// retirement keeps the values. A colleague's agent holds no such grant.
func TestAnAgentAddsACustomFieldOnlyWithTheUsersGrant(t *testing.T) {
	const adminEmail = "agent-field@fable.test"
	d := newTwoDoorsWith(t, "agent-field", []compose.Option{compose.WithSchemaPool(SchemaPool(t))}, "read", "write", "send")
	created := settingsAct(t, d, "create_custom_field", map[string]any{"request_body": AnyMap{
		"object": "company", "label": "Region code", "type": "text", "source": "manual",
	}})
	id := answerID(t, created)
	if got := actorOfLastAudit(t, d, "custom_field", id); got != "agent" {
		t.Errorf("the field's audit row names %q, want the agent", got)
	}
	var fields json.RawMessage
	if status := d.e.Call(t, "GET", "/v1/custom-fields?object=company", nil, nil, &fields); status != http.StatusOK || !strings.Contains(string(fields), id) {
		t.Errorf("the user reading the fields → %d, without %s", status, id)
	}
	colleague := colleagueDoors(t, d, adminEmail)
	got := colleague.mcp.Call(t, "change_settings", map[string]any{"action": "create_custom_field", "request_body": AnyMap{
		"object": "company", "label": "Rep's own", "type": "text", "source": "manual",
	}})
	if !got.IsError || !strings.Contains(got.Text, "permission denied") {
		t.Errorf("a rep's agent creating a field answered %+v, want permission denied", got)
	}
}

func withIfMatch(headers map[string]string, version int64) map[string]string {
	out := maps.Clone(headers)
	if out == nil {
		out = map[string]string{}
	}
	out["If-Match"] = strconv.FormatInt(version, 10)
	return out
}

// An occupied position names itself and what to do, on both doors.
func TestATakenStagePositionSaysWhichAndWhatToDo(t *testing.T) {
	d := newTwoDoors(t, "stage-position-taken", "read", "write")
	pipeline := answerID(t, settingsAct(t, d, "create_pipeline", map[string]any{"request_body": AnyMap{
		"name": "Crowded", "stages": []AnyMap{{"name": "First", "position": 1, "semantic": "open"}, {"name": "Second", "position": 2, "semantic": "open"}},
	}}))
	taken := AnyMap{"pipeline_id": pipeline, "name": "Late", "position": 2}
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status := d.e.Call(t, "POST", "/v1/stages", taken, nil, &problem); status != http.StatusConflict {
		t.Fatalf("the user taking an occupied position → %d", status)
	}
	if problem.Code != "stage_position_taken" || !strings.Contains(problem.Detail, "position 2") || !strings.Contains(problem.Detail, "reorder") {
		t.Errorf("the user's door answered %+v", problem)
	}
	refusal := d.toolRefused(t, "change_settings", map[string]any{"action": "create_stage", "request_body": taken})
	if !strings.Contains(refusal, "position 2") || !strings.Contains(refusal, "reorder") {
		t.Errorf("the agent's door answered %q", refusal)
	}
	var list struct {
		Stages []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stages"`
	}
	if status := d.e.Call(t, "GET", "/v1/pipelines/"+pipeline, nil, nil, &list); status != http.StatusOK {
		t.Fatalf("reading the pipeline → %d", status)
	}
	for _, stage := range list.Stages {
		if stage.Name != "First" {
			continue
		}
		var moved struct {
			Detail string `json:"detail"`
		}
		if status := d.e.Call(t, "PATCH", "/v1/stages/"+stage.ID, AnyMap{"position": 2}, nil, &moved); status != http.StatusConflict || !strings.Contains(moved.Detail, "position 2") {
			t.Errorf("moving onto an occupied position → %d %q", status, moved.Detail)
		}
	}
}
