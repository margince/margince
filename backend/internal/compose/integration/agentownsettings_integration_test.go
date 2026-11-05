// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func ownAct(t *testing.T, d *twoDoors, action string, args map[string]any) json.RawMessage {
	t.Helper()
	args["action"] = action
	return d.tool(t, "change_my_settings", args)
}

// Each own setting lands on the agent's user, as the user's own door reads it,
// and its audit row names the agent.
func TestAnAgentChangesItsOwnUsersSettingsAndIsNamedForIt(t *testing.T) {
	const email = "agent-own@fable.test"
	d := newTwoDoors(t, "agent-own", "read", "write")
	user := memberID(t, d, email)
	auditedBy := func(what string) {
		t.Helper()
		var actorType string
		if err := d.e.Owner.QueryRow(t.Context(), `
			SELECT actor_type FROM audit_log WHERE entity_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1`, user).Scan(&actorType); err != nil {
			t.Fatalf("reading the audit row of %s: %v", what, err)
		}
		if actorType != "agent" {
			t.Errorf("%s: the audit row names %q, want the agent", what, actorType)
		}
	}

	ownAct(t, d, "save_display_name", map[string]any{"request_body": AnyMap{"display_name": "Renamed By Agent"}})
	auditedBy("the display name")
	ownAct(t, d, "save_locale", map[string]any{"request_body": AnyMap{"locale": "de"}})
	auditedBy("the language")
	ownAct(t, d, "save_timezone", map[string]any{"request_body": AnyMap{"timezone": "Europe/Berlin", "source": "browser"}})
	auditedBy("the time zone")
	var displayName, locale string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT display_name, coalesce(locale, '') FROM app_user WHERE id = $1`, user).Scan(&displayName, &locale); err != nil {
		t.Fatalf("reading the seat: %v", err)
	}
	if displayName != "Renamed By Agent" || locale != "de" {
		t.Errorf("the seat reads name %q, language %q", displayName, locale)
	}

	for topic, path := range map[string]string{"my_working_hours": "/v1/me/working-hours", "my_brief_delivery": "/v1/me/brief-delivery"} {
		var viaREST json.RawMessage
		if status := d.e.Call(t, "GET", path, nil, nil, &viaREST); status != http.StatusOK {
			t.Fatalf("GET %s → %d", path, status)
		}
		sameRecords[map[string]any](t, topic, viaREST, d.tool(t, "read_workspace", map[string]any{"topic": topic}))
	}
}

// What the product tells a user about their agent is the user's alone. So is how
// they sign in: the seat lookup that opened for own settings must not open these.
func TestAnAgentNeverDecidesWhatReachesItsUserNorTheirSignIn(t *testing.T) {
	d := newTwoDoors(t, "agent-own-limits", "read", "write", "send", "enrich")
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"PUT", "/v1/me/brief-delivery", AnyMap{}},
		{"PUT", "/v1/me/notification-preferences", AnyMap{"class": "approval", "delivery": "off"}},
		{"GET", "/v1/me/sessions", nil},
		{"GET", "/v1/me/mfa", nil},
		{"POST", "/v1/ai/feedback", AnyMap{}},
	} {
		if status := d.e.Call(t, call.method, call.path, call.body, d.bearer, nil); status != http.StatusForbidden {
			t.Errorf("%s %s as a passport → %d, want 403", call.method, call.path, status)
		}
	}
}

// The voice an agent sets up belongs to its user, who reads it on their own door.
// Their words stay theirs: a colleague's agent finds nothing there.
func TestAnAgentSetsUpItsOwnUsersVoice(t *testing.T) {
	const adminEmail = "agent-voice@fable.test"
	d := newTwoDoors(t, "agent-voice", "read", "write", "send")
	profile := decodeVersioned(t, string(ownAct(t, d, "create_voice_profile", map[string]any{"request_body": AnyMap{"personality_md": "Short sentences."}})))
	if status := d.e.Call(t, "GET", "/v1/voice-profiles/"+profile.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("the user reading their voice → %d", status)
	}
	var capturedBy string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT captured_by FROM voice_profile WHERE id = $1`, profile.ID).Scan(&capturedBy); err != nil {
		t.Fatalf("reading the profile: %v", err)
	}
	if !isAgentStamp(capturedBy) {
		t.Errorf("the profile was captured by %q, want the agent", capturedBy)
	}
	source := ownAct(t, d, "add_voice_source", map[string]any{"id": profile.ID, "request_body": AnyMap{
		"kind": "other", "source_label": "A note", "register": "general", "source_ref": "note-1", "format": "text",
		"content": "I keep my notes short and plain, and I say what I mean.",
	}})
	if !strings.Contains(string(source), "note-1") {
		t.Errorf("adding a sample answered %s", source)
	}
	colleague := colleagueDoors(t, d, adminEmail)
	got := colleague.mcp.Call(t, "change_my_settings", map[string]any{
		"action": "add_voice_source", "id": profile.ID, "request_body": AnyMap{
			"kind": "other", "source_label": "Theirs", "register": "general", "source_ref": "note-2", "format": "text", "content": "A colleague's words.",
		},
	})
	if !got.IsError {
		t.Errorf("a colleague's agent added a sample to this voice: %+v", got)
	}
	for _, path := range []string{"/corpus/clear", "/draft-rejections"} {
		if status := d.e.Call(t, "POST", "/v1/voice-profiles/"+profile.ID+path, AnyMap{}, d.bearer, nil); status != http.StatusForbidden {
			t.Errorf("POST %s as a passport → %d, want 403", path, status)
		}
	}
}

// Marking a record seen spends the user's own since-last-visit, so it is the
// action the tool says to ask about. It moves their marker and names the agent.
func TestAnAgentMarkingACompanySeenMovesItsUsersMarker(t *testing.T) {
	d := newTwoDoors(t, "agent-view-ack", "read", "write")
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Seen Co"})
	d.tool(t, "change_attention", map[string]any{"action": "acknowledge_company_view", "id": company})
	var seen int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM user_record_view WHERE entity_id = $1`, company).Scan(&seen); err != nil {
		t.Fatalf("reading the marker: %v", err)
	}
	if seen != 1 {
		t.Errorf("the user's marker rows = %d, want 1", seen)
	}
}
