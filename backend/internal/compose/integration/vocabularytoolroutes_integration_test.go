// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The agent tools that take no arguments and describe a grammar or the caller,
// each served on its own route by the registry. A route and its tool answer one
// JSON document byte for byte.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestVocabularyRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "vocabulary-routes", "read")
	for _, tc := range []struct{ path, tool, member string }{
		{"/v1/analytics/vocabulary", "describe_analytics_vocabulary", `"vocabulary":"`},
		{"/v1/record-fields", "describe_record_fields", `"fields":{`},
		{"/v1/analytics/reports/blocks", "describe_report_blocks", `"blocks":{`},
		{"/v1/analytics/reports/vocabulary", "describe_report_vocabulary", `"vocabulary":{`},
	} {
		rest := compacted(t, d.rest(t, "GET", tc.path, nil))
		if rest != compacted(t, d.tool(t, tc.tool, map[string]any{})) {
			t.Errorf("GET %s and %s answered different documents", tc.path, tc.tool)
		}
		if !strings.Contains(rest, tc.member) {
			t.Errorf("GET %s carries no %s member: %.200s", tc.path, tc.member, rest)
		}
	}
}

// GET /whoami names the admin the passport acts for, as whoami does, and a
// session reads the same person for itself.
func TestWhoamiRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "whoami-route", "read")
	rest := d.rest(t, "GET", "/v1/whoami", nil)
	if compacted(t, rest) != compacted(t, d.tool(t, "whoami", map[string]any{})) {
		t.Errorf("GET /v1/whoami and whoami answered different documents:\n%s", rest)
	}

	var session json.RawMessage
	if status := d.e.Call(t, "GET", "/v1/whoami", nil, nil, &session); status != http.StatusOK {
		t.Fatalf("a session's GET /v1/whoami → %d %s", status, session)
	}
	var forAgent, forSession struct {
		ActingUserID string `json:"acting_user_id"`
		Email        string `json:"email"`
	}
	if json.Unmarshal(rest, &forAgent) != nil || json.Unmarshal(session, &forSession) != nil {
		t.Fatalf("the identities do not decode: %s / %s", rest, session)
	}
	if forAgent.ActingUserID == "" || forAgent != forSession || forAgent.Email != "whoami-route@fable.test" {
		t.Errorf("the passport acts for %+v and the session is %+v, want the one admin who issued it", forAgent, forSession)
	}
}
