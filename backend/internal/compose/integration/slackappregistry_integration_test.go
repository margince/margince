// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestSlackApplicationParticipatesInInstallationSetup(t *testing.T) {
	e := setupConnectorAppHTTP(t)
	secret, signing := "synthetic-slack-client-secret", "0123456789abcdef0123456789abcdef"
	input := crmcontracts.ConnectorAppInput{ClientId: "123.456", ClientSecret: &secret, SigningSecret: &signing}
	if status := e.Call(t, http.MethodPut, "/v1/installation/oauth-apps/slack", input, nil, nil); status != http.StatusNoContent {
		t.Fatalf("save Slack app=%d", status)
	}
	var app crmcontracts.ConnectorApp
	if status := e.Call(t, http.MethodGet, "/v1/installation/oauth-apps/slack", nil, nil, &app); status != http.StatusOK || !app.Configured || app.ClientId != input.ClientId {
		t.Fatalf("read Slack app=%d %+v", status, app)
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/setup", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("setup with Slack app=%d", status)
	}
}
