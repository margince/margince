// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A sample the user filed or left out stays as they decided. An agent cannot
// replace its text under the same reference or put back what they excluded.
func TestAnAgentKeepsTheVoiceSamplesTheUserDecided(t *testing.T) {
	d := newTwoDoors(t, "agent-voice-decided", "read", "write")
	var profile struct {
		ID string `json:"id"`
	}
	if status := d.e.Call(t, "POST", "/v1/voice-profiles", AnyMap{}, nil, &profile); status != http.StatusCreated {
		t.Fatalf("creating the profile → %d", status)
	}
	sample := func(ref, content string) AnyMap {
		return AnyMap{"kind": "other", "source_label": "A note", "register": "general", "source_ref": ref, "format": "text", "content": content}
	}
	const mine = "These are the words I wrote myself and want kept exactly so."
	var filed struct {
		Source struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"source"`
	}
	if status := d.e.Call(t, "POST", "/v1/voice-profiles/"+profile.ID+"/sources", sample("mine", mine), nil, &filed); status != http.StatusCreated {
		t.Fatalf("filing the sample → %d", status)
	}
	source := filed.Source
	if status := d.e.Call(t, "PATCH", "/v1/voice-profiles/"+profile.ID+"/sources/"+source.ID, AnyMap{"included": false},
		withIfMatch(nil, source.Version), &filed); status != http.StatusOK {
		t.Fatalf("excluding the sample → %d", status)
	}
	source = filed.Source
	refusal := d.toolRefused(t, "change_my_settings", map[string]any{
		"action": "add_voice_source", "id": profile.ID, "request_body": sample("mine", "Words an agent put in the user's mouth instead."),
	})
	if !strings.Contains(refusal, "does not replace") {
		t.Errorf("replacing the sample answered %q", refusal)
	}
	refusal = d.toolRefused(t, "change_my_settings", map[string]any{
		"action": "update_voice_source", "id": profile.ID, "sourceId": source.ID, "if_version": source.Version, "request_body": AnyMap{"included": true},
	})
	if !strings.Contains(refusal, "left out") {
		t.Errorf("putting the sample back answered %q", refusal)
	}
	var content string
	var excluded bool
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT content, excluded FROM voice_corpus_source WHERE id = $1`, source.ID).Scan(&content, &excluded); err != nil {
		t.Fatalf("reading the sample: %v", err)
	}
	if content != mine || !excluded {
		t.Errorf("the sample now reads %q, excluded %t", content, excluded)
	}
	ownAct(t, d, "add_voice_source", map[string]any{"id": profile.ID, "request_body": sample("agents", "A new sample under its own reference is fine to add.")})
}

// What a web form promises a visitor, and where it takes posts from, stays with
// a user. Sending the value it already holds is no change. The audit keeps the wording.
func TestAnAgentChangesNoFormConsentOrOrigin(t *testing.T) {
	d := newTwoDoors(t, "agent-form-consent", "read", "write")
	site := []string{"https://site.example"}
	var form struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := d.e.Call(t, "POST", "/v1/web-forms", AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": site}, nil, &form); status != http.StatusCreated {
		t.Fatalf("creating the form → %d", status)
	}
	for name, body := range map[string]AnyMap{
		"origins":        {"allowed_origins": []string{"https://elsewhere.example"}},
		"consent switch": {"consent_required": true, "consent_wording": "You agree we may write to you."},
		"wording":        {"consent_wording": "You agree to anything."},
		"policy":         {"consent_policy_version": "v9"},
		"notice":         {"privacy_notice": "We keep it forever."},
	} {
		refusal := d.toolRefused(t, "change_settings", map[string]any{"action": "update_web_form", "id": form.ID, "if_version": form.Version, "request_body": body})
		if !strings.Contains(refusal, "an agent does not change") {
			t.Errorf("%s answered %q", name, refusal)
		}
	}
	settingsAct(t, d, "update_web_form", map[string]any{"id": form.ID, "if_version": form.Version, "request_body": AnyMap{"allowed_origins": site, "name": "Contact us"}})

	var after struct {
		Version int64 `json:"version"`
	}
	if status := d.e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, AnyMap{"consent_wording": "We may write to you."},
		withIfMatch(nil, form.Version+1), &after); status != http.StatusOK {
		t.Fatalf("the user changing the wording → %d", status)
	}
	var recorded string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT coalesce(after->>'consent_wording', '') FROM audit_log WHERE entity_id = $1
		ORDER BY occurred_at DESC, id DESC LIMIT 1`, form.ID).Scan(&recorded); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if recorded != "We may write to you." {
		t.Errorf("the audit row records the wording as %q", recorded)
	}
}

// An allowlist means the same in any order, so an agent resending it reordered
// changes no promise and its other edit goes through.
func TestAnAgentResendingAFormsOriginsInAnotherOrderIsNoChange(t *testing.T) {
	d := newTwoDoors(t, "agent-form-origins-order", "read", "write")
	var form struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	created := AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{"https://a.example", "https://b.example"}}
	if status := d.e.Call(t, "POST", "/v1/web-forms", created, nil, &form); status != http.StatusCreated {
		t.Fatalf("creating the form → %d", status)
	}
	settingsAct(t, d, "update_web_form", map[string]any{
		"id": form.ID, "if_version": form.Version,
		"request_body": AnyMap{"allowed_origins": []string{"https://b.example", "https://a.example"}, "name": "Contact us"},
	})
	var name string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT name FROM web_form WHERE id = $1`, form.ID).Scan(&name); err != nil {
		t.Fatalf("reading the form: %v", err)
	}
	if name != "Contact us" {
		t.Errorf("the form is named %q", name)
	}
}

// automationRow is one rule as the user's own door lists it.
type automationRow struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

func automationKeyed(t *testing.T, d *twoDoors, key string) automationRow {
	t.Helper()
	var list struct {
		Data []automationRow `json:"data"`
	}
	if status := d.e.Call(t, "GET", "/v1/automations", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("listing the automations → %d", status)
	}
	for _, row := range list.Data {
		if row.Key == key {
			return row
		}
	}
	t.Fatalf("no automation %s in %+v", key, list.Data)
	return automationRow{}
}

// An agent cannot retune or remove an unreviewed starter. A user reviews it first.
func TestAnAgentCannotQuietASeededRule(t *testing.T) {
	d := newTwoDoors(t, "agent-seeded-rule", "read", "write")
	rule := automationKeyed(t, d, "stage_change_notify")
	if rule.Status != "paused" {
		t.Fatalf("the unreviewed rule starts %q", rule.Status)
	}
	stages := apptest.DefaultPipeline(t, d.e)
	params := AnyMap{"pipeline_id": stages.PipelineID, "stage_ids": []string{stages.Open}}
	refusal := d.toolRefused(t, "change_settings", map[string]any{"action": "update_automation", "id": rule.ID, "if_version": rule.Version, "request_body": AnyMap{"params": params}})
	if !strings.Contains(refusal, "seeded") {
		t.Errorf("retuning answered %q", refusal)
	}
	refusal = d.toolRefused(t, "change_settings", map[string]any{"action": "archive_automation", "id": rule.ID})
	if !strings.Contains(refusal, "seeded") {
		t.Errorf("archiving answered %q", refusal)
	}
	if got := automationKeyed(t, d, "stage_change_notify"); got.Status != "paused" || got.Version != rule.Version {
		t.Errorf("the unreviewed rule changed: %+v", got)
	}
	if status := d.e.Call(t, "PATCH", "/v1/automations/"+rule.ID, AnyMap{"status": "enabled", "params": params}, withIfMatch(nil, rule.Version), nil); status != http.StatusOK {
		t.Fatalf("the user reviewing the rule: %d", status)
	}
	reviewed := automationKeyed(t, d, "stage_change_notify")
	if reviewed.Status != "enabled" {
		t.Fatalf("the reviewed rule is %q", reviewed.Status)
	}
	if status := d.e.Call(t, "PATCH", "/v1/automations/"+rule.ID, AnyMap{"status": "paused"}, withIfMatch(nil, reviewed.Version), nil); status != http.StatusOK {
		t.Errorf("the user pausing their rule: %d", status)
	}
}

// A rule runs with its owner's authority, so an agent changes no rule's owner.
func TestAnAgentHandsNoRuleToAColleague(t *testing.T) {
	const adminEmail = "agent-rule-owner@fable.test"
	d := newTwoDoors(t, "agent-rule-owner", "read", "write", "send")
	user := memberID(t, d, adminEmail)
	var colleague string
	if err := d.e.Owner.QueryRow(t.Context(), `
		INSERT INTO app_user (email, display_name) VALUES ('rule-rep@colleague.test', 'Rule Rep') RETURNING id::text`).Scan(&colleague); err != nil {
		t.Fatalf("seeding the colleague: %v", err)
	}
	refused := d.toolRefused(t, "change_settings", map[string]any{"action": "create_automation", "request_body": AnyMap{
		"key": "assign_lead_owner", "name": "Named owner", "params": AnyMap{}, "owner_id": colleague,
	}})
	if !strings.Contains(refused, "owner_id") {
		t.Errorf("naming an owner on create answered %q", refused)
	}
	created := settingsAct(t, d, "create_automation", map[string]any{"request_body": AnyMap{
		"key": "assign_lead_owner", "name": "Agent-made", "params": AnyMap{},
	}})
	var rule struct {
		ID      string `json:"id"`
		OwnerID string `json:"owner_id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(created, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.OwnerID != user {
		t.Errorf("a rule an agent created belongs to %q, want its user %q", rule.OwnerID, user)
	}
	refusal := d.toolRefused(t, "change_settings", map[string]any{"action": "update_automation", "id": rule.ID, "if_version": rule.Version, "request_body": AnyMap{"owner_id": colleague}})
	if !strings.Contains(refusal, "an agent does not change who owns") {
		t.Errorf("handing the rule on answered %q", refusal)
	}
}

// A recap rule composes each draft with a model, so enabling it spends AI budget
// from then on. A passport without enrich cannot switch that on.
func TestEnablingARuleThatSpendsAModelNeedsEnrich(t *testing.T) {
	d := newTwoDoors(t, "agent-rule-enrich", "read", "write")
	rule := automationKeyed(t, d, "post_meeting_recap")
	args := map[string]any{"action": "update_automation", "id": rule.ID, "if_version": rule.Version, "request_body": AnyMap{"status": "enabled"}}
	refusal := d.toolRefused(t, "change_settings", args)
	if !strings.Contains(refusal, "enrich") {
		t.Errorf("enabling without enrich answered %q", refusal)
	}
	if got := automationKeyed(t, d, "post_meeting_recap"); got.Status == "enabled" {
		t.Errorf("the rule was enabled: %+v", got)
	}
	bearer, _ := apptest.PassportWithID(t, d.e, "with enrich", "read", "write", "enrich")
	withEnrich := &twoDoors{e: d.e, bearer: bearer, mcp: apptest.NewMCPClient(d.e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))}
	withEnrich.tool(t, "change_settings", args)
	if got := automationKeyed(t, d, "post_meeting_recap"); got.Status != "enabled" {
		t.Errorf("with enrich the rule reads %q", got.Status)
	}
}

// A permanent delete stays with a user, on either door.
func TestAnAgentDeletesNoPartnerStageOrTranslation(t *testing.T) {
	d := newTwoDoors(t, "agent-no-delete", "read", "write", "send", "enrich")
	placeholder := placeholderFor("id")
	for _, path := range []string{"/v1/partner-stages/" + placeholder, "/v1/products/" + placeholder + "/translations/de"} {
		if status := d.e.Call(t, "DELETE", path, nil, d.bearer, nil); status != http.StatusForbidden {
			t.Errorf("DELETE %s as a passport → %d, want 403", path, status)
		}
	}
	for _, action := range []string{"delete_partner_stage", "delete_product_translation"} {
		d.mcp.CallRefused(t, "change_settings", map[string]any{"action": action, "id": placeholder})
	}
}

// "Set my time zone to Berlin" takes effect through an agent, and a later browser
// guess leaves it. The row and the audit name the agent, until the user chooses.
func TestATimeZoneAnAgentSetsHoldsAndNamesIt(t *testing.T) {
	const email = "agent-zone@fable.test"
	d := newTwoDoors(t, "agent-zone", "read", "write")
	user := memberID(t, d, email)
	seatZone := func() (zone string, chosen bool, setBy *string) {
		t.Helper()
		if err := d.e.Owner.QueryRow(t.Context(), `SELECT timezone, timezone_chosen, timezone_set_by FROM app_user WHERE id = $1`, user).Scan(&zone, &chosen, &setBy); err != nil {
			t.Fatalf("reading the seat: %v", err)
		}
		return zone, chosen, setBy
	}
	for _, source := range []string{"chosen", "browser"} {
		refusal := d.toolRefused(t, "change_my_settings", map[string]any{"action": "save_timezone", "request_body": AnyMap{"timezone": "Asia/Tokyo", "source": source}})
		if !strings.Contains(refusal, "an agent does not choose") {
			t.Errorf("sending source %s answered %q", source, refusal)
		}
	}
	ownAct(t, d, "save_timezone", map[string]any{"request_body": AnyMap{"timezone": "Europe/Berlin", "source": "agent"}})
	zone, chosen, setBy := seatZone()
	if zone != "Europe/Berlin" || chosen || setBy == nil || !isAgentStamp(*setBy) {
		t.Errorf("the seat reads %q, chosen %t, set by %v", zone, chosen, setBy)
	}
	if got := actorOfLastAudit(t, d, "user", user); got != "agent" {
		t.Errorf("the audit row names %q, want the agent", got)
	}
	if status := d.e.Call(t, "PUT", "/v1/me/timezone", AnyMap{"timezone": "America/New_York", "source": "browser"}, nil, nil); status != http.StatusOK {
		t.Errorf("the browser's guess → %d", status)
	}
	if zone, _, _ := seatZone(); zone != "Europe/Berlin" {
		t.Errorf("the browser's guess moved the zone to %q", zone)
	}
	if status := d.e.Call(t, "PUT", "/v1/me/timezone", AnyMap{"timezone": "Europe/Berlin", "source": "chosen"}, nil, nil); status != http.StatusOK {
		t.Errorf("the user choosing the same zone → %d", status)
	}
	if _, chosen, setBy := seatZone(); !chosen || setBy != nil {
		t.Errorf("after the user chose it, chosen %t, set by %v", chosen, setBy)
	}
	if status := d.e.Call(t, "PUT", "/v1/me/timezone", AnyMap{"timezone": "Europe/Berlin", "source": "agent"}, nil, nil); status == http.StatusOK {
		t.Errorf("a user's request claimed to be an agent's")
	}
}

// Working hours carry a time zone too, and that door names the agent the same way.
func TestATimeZoneSentWithWorkingHoursNamesTheAgent(t *testing.T) {
	const email = "agent-hours-zone@fable.test"
	d := newTwoDoors(t, "agent-hours-zone", "read", "write")
	user := memberID(t, d, email)
	hours := AnyMap{"start_time": "09:00", "end_time": "17:00", "days": []int{1, 2, 3, 4}, "timezone": "Asia/Tokyo"}
	ownAct(t, d, "save_working_hours", map[string]any{"request_body": hours})
	var setBy *string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT timezone_set_by FROM app_user WHERE id = $1`, user).Scan(&setBy); err != nil {
		t.Fatalf("reading the seat: %v", err)
	}
	if setBy == nil || !isAgentStamp(*setBy) {
		t.Errorf("the zone reads set by %v, want the agent", setBy)
	}
}

// A signal an agent resolves does not read as resolved by its user. The row names
// the agent, as part of what the audit already says.
func TestASignalAnAgentResolvesDoesNotNameTheUser(t *testing.T) {
	d := newTwoDoors(t, "agent-signal", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Signal Co"})
	var signal struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := d.e.Call(t, "POST", "/v1/signals", AnyMap{
		"kind": "reengagement", "summary": "They wrote back", "source": "manual", "entity_type": "company", "entity_id": company,
	}, nil, &signal); status != http.StatusCreated {
		t.Fatalf("recording the signal → %d", status)
	}
	answer := d.tool(t, "change_attention", map[string]any{"action": "update_signal", "id": signal.ID, "if_version": signal.Version, "request_body": AnyMap{"status": "resolved", "note": "Done."}})
	if strings.Contains(string(answer), `"resolved_by":"`) {
		t.Errorf("the answer names a user as having resolved it: %s", answer)
	}
	var resolvedBy *string
	var capturedBy string
	if err := d.e.Owner.QueryRow(t.Context(), `
		SELECT resolved_by::text, captured_by FROM signal_resolution WHERE signal_id = $1 AND outcome = 'resolved'`, signal.ID).Scan(&resolvedBy, &capturedBy); err != nil {
		t.Fatalf("reading the outcome: %v", err)
	}
	if resolvedBy != nil || !isAgentStamp(capturedBy) {
		t.Errorf("the outcome reads resolved_by %v, captured_by %q", resolvedBy, capturedBy)
	}
	// The tool does not offer the kind, and a body that sets it anyway is refused at the door.
	refusal := d.toolRefused(t, "change_attention", map[string]any{"action": "update_signal", "id": signal.ID, "request_body": AnyMap{"kind": "risk"}})
	if !strings.Contains(refusal, "only a human may set kind") {
		t.Errorf("an agent moving the kind answered %q, want the human-only refusal", refusal)
	}
}

// A transition trusted to move deals by itself judges a move by its stage's
// criteria and by which stage comes next. An agent cannot change either under it.
func TestAnAgentReshapesNoLadderATransitionIsTrustedOn(t *testing.T) {
	d := newTwoDoors(t, "agent-trusted-ladder", "read", "write")
	var pipeline struct {
		ID     string `json:"id"`
		Stages []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Position int    `json:"position"`
		} `json:"stages"`
		Version int64 `json:"version"`
	}
	if status := d.e.Call(t, "POST", "/v1/pipelines", AnyMap{"name": "Trusted", "stages": []AnyMap{
		{"name": "One", "position": 1, "semantic": "open"},
		{"name": "Two", "position": 2, "semantic": "open"},
		{"name": "Three", "position": 3, "semantic": "open"},
		{"name": "Won", "position": 4, "semantic": "won"},
	}}, nil, &pipeline); status != http.StatusCreated {
		t.Fatalf("creating the pipeline → %d", status)
	}
	stage := func(name string) string {
		for _, s := range pipeline.Stages {
			if s.Name == name {
				return s.ID
			}
		}
		t.Fatalf("no stage %s", name)
		return ""
	}
	var criterion struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := d.e.Call(t, "POST", "/v1/stages/"+stage("One")+"/exit-criteria", AnyMap{"key": "budget", "label": "Budget known", "kind": "custom", "required": true}, nil, &criterion); status != http.StatusCreated {
		t.Fatalf("adding the criterion → %d", status)
	}
	order := []string{stage("One"), stage("Three"), stage("Two"), stage("Won")}
	calls := map[string]map[string]any{
		"archiving a criterion": {"action": "archive_exit_criterion", "id": stage("One"), "criterion_id": criterion.ID},
		"making it optional":    {"action": "update_exit_criterion", "id": stage("One"), "criterion_id": criterion.ID, "request_body": AnyMap{"required": false}},
		"reordering the stages": {"action": "reorder_stages", "id": pipeline.ID, "request_body": AnyMap{"stage_ids": order}},
		"moving a stage":        {"action": "update_stage", "id": stage("Two"), "request_body": AnyMap{"position": 9}},
		"removing a stage":      {"action": "archive_stage", "id": stage("Two")},
		"adding a stage":        {"action": "create_stage", "request_body": AnyMap{"pipeline_id": pipeline.ID, "name": "Inserted", "position": 8}},
		"adding a criterion":    {"action": "create_exit_criterion", "id": stage("Two"), "request_body": AnyMap{"key": "extra", "label": "Extra", "kind": "custom", "required": true}},
	}
	trustTransition(t, d, pipeline.ID, stage("One"), stage("Three"))
	for name, args := range calls {
		refusal := d.toolRefused(t, "change_settings", args)
		if !strings.Contains(refusal, "trusted") {
			t.Errorf("%s answered %q", name, refusal)
		}
	}
	if _, err := d.e.Owner.Exec(t.Context(), `UPDATE stage_progression_policy SET mode = 'propose' WHERE pipeline_id = $1`, pipeline.ID); err != nil {
		t.Fatalf("taking the trust back: %v", err)
	}
	settingsAct(t, d, "update_exit_criterion", map[string]any{"id": stage("One"), "criterion_id": criterion.ID, "request_body": AnyMap{"required": false}})
}

// A captured call holds what was sent to a model and what came back, which reach
// past the user's row scope. An agent reads the call and not the payload.
func TestAnAgentReadsAnAICallWithoutItsPayload(t *testing.T) {
	d := newTwoDoors(t, "agent-ai-call", "read", "write")
	var call string
	if err := d.e.Owner.QueryRow(t.Context(), `
		INSERT INTO ai_call (task, tier, provider, model_id, request_fingerprint, logical_call_id)
		VALUES ('cold_start', 'cheap_cloud', 'anthropic', 'a-model', 'print', gen_random_uuid()) RETURNING id::text`).Scan(&call); err != nil {
		t.Fatalf("seeding the call: %v", err)
	}
	if _, err := d.e.Owner.Exec(t.Context(), `
		INSERT INTO ai_call_payload (ai_call_id, request_payload, response_payload)
		VALUES ($1, '{"secret":"what the prompt held"}', '{"secret":"what the model said"}')`, call); err != nil {
		t.Fatalf("seeding the payload: %v", err)
	}
	var viaREST json.RawMessage
	if status := d.e.Call(t, "GET", "/v1/ai/calls/"+call, nil, nil, &viaREST); status != http.StatusOK || !strings.Contains(string(viaREST), "what the prompt held") {
		t.Fatalf("the user reads the call as %d %s", status, viaREST)
	}
	got := string(d.tool(t, "read_workspace", map[string]any{"topic": "ai_call", "id": call}))
	if strings.Contains(got, "what the prompt held") || strings.Contains(got, "what the model said") {
		t.Errorf("the agent read the captured payload: %s", got)
	}
	if !strings.Contains(got, "a-model") {
		t.Errorf("the agent read nothing of the call: %s", got)
	}
}

// trustTransition seeds the row directly because no route writes this table any more,
// and a row enabled while one did still holds its ladder still.
func trustTransition(t *testing.T, d *twoDoors, pipeline, from, to string) {
	t.Helper()
	if _, err := d.e.Owner.Exec(t.Context(), `
		INSERT INTO stage_progression_policy (pipeline_id, from_stage_id, to_stage_id, mode)
		VALUES ($1, $2, $3, 'auto')`, pipeline, from, to); err != nil {
		t.Fatalf("trusting the transition: %v", err)
	}
}

// A transition whose stage was removed moves no deal, so it leaves the ladder free.
func TestAnAgentReshapesALadderOnceItsTrustedStageIsGone(t *testing.T) {
	d := newTwoDoors(t, "agent-trust-gone", "read", "write")
	var pipeline struct {
		ID     string `json:"id"`
		Stages []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stages"`
	}
	if status := d.e.Call(t, "POST", "/v1/pipelines", AnyMap{"name": "Gone", "stages": []AnyMap{
		{"name": "One", "position": 1, "semantic": "open"},
		{"name": "Two", "position": 2, "semantic": "open"},
		{"name": "Won", "position": 3, "semantic": "won"},
	}}, nil, &pipeline); status != http.StatusCreated {
		t.Fatalf("creating the pipeline → %d", status)
	}
	trustTransition(t, d, pipeline.ID, pipeline.Stages[0].ID, pipeline.Stages[1].ID)
	if status := d.e.Call(t, "DELETE", "/v1/stages/"+pipeline.Stages[1].ID, nil, nil, nil); status >= 300 {
		t.Fatalf("removing the stage → %d", status)
	}
	settingsAct(t, d, "create_stage", map[string]any{"request_body": AnyMap{"pipeline_id": pipeline.ID, "name": "Three", "position": 7}})
}
