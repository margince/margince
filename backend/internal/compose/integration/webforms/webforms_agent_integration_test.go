// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// An agent sets a form up like its user, and no signing secret enters its answer
// on either door. The secret and the form's websites stay its user's to set.
// The user then mints a secret in the app and the website signs with it.
func TestAnAgentMakesAServerFormWithoutEverSeeingItsSecret(t *testing.T) {
	t.Parallel()
	d := apptest.SetupTwoDoors(t, "agent-web-form", []compose.Option{compose.WithKeyvault(keyvault.NewMemory())}, "read", "write")
	raw := d.MCP.CallOK(t, "change_settings", map[string]any{
		"action":       "create_web_form",
		"request_body": map[string]any{"name": "Partner site", "sender": "server"},
	}).Envelope(t).Data
	var viaTool struct {
		createdForm
		SecretConfigured bool `json:"secret_configured"`
	}
	if err := json.Unmarshal(raw, &viaTool); err != nil {
		t.Fatalf("the tool's answer does not decode: %v\n%s", err, raw)
	}
	if viaTool.SigningSecret != nil || !viaTool.SecretConfigured {
		t.Fatalf("tool answer = %s, want a form whose secret is set and not shown", raw)
	}

	var viaREST createdForm
	body := integration.AnyMap{"name": "Partner site two", "sender": "server"}
	if status := d.E.Call(t, "POST", "/v1/web-forms", body, d.Bearer, &viaREST); status != http.StatusCreated || viaREST.SigningSecret != nil {
		t.Fatalf("create as a passport → %d, secret %v; want 201 and no secret", status, viaREST.SigningSecret)
	}

	for what, refused := range map[string]integration.AnyMap{
		"a secret it chose":             {"name": "Webflow", "sender": "webflow", "sender_secret": "agent-chosen-secret-1"},
		"a secret no server form takes": {"name": "Backend", "sender": "server", "sender_secret": "agent-chosen-secret-2"},
		"its websites":                  {"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}},
		"websites no server form takes": {"name": "Backend", "sender": "server", "allowed_origins": []string{siteOrigin}},
	} {
		if status := d.E.Call(t, "POST", "/v1/web-forms", refused, d.Bearer, nil); status != http.StatusForbidden {
			t.Errorf("an agent's form with %s → %d, want 403: that stays with its user", what, status)
		}
	}

	var rotated createdForm
	if status := d.E.Call(t, "POST", "/v1/web-forms/"+viaTool.ID+"/rotate-secret", nil, nil, &rotated); status != http.StatusOK || rotated.SigningSecret == nil {
		t.Fatalf("the user's Make a new secret → %d, secret %v", status, rotated.SigningSecret)
	}
	post := []byte(`{"email":"pia@visitor.example","message":"Sent from the partner site"}`)
	signed := serverHeaders(*rotated.SigningSecret, viaTool.FormKey, post, "c0ffee01")
	if answer := postForm(t, d.E, "POST", viaTool.FormKey, "application/json", post, signed); answer.Status != http.StatusAccepted {
		t.Fatalf("a post signed with the user's secret → %d %s", answer.Status, answer.Body)
	}
}
