// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

type userAuthorityTool struct {
	fakeTool
	seen chan context.Context
}

func (tool *userAuthorityTool) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	tool.seen <- ctx
	return json.RawMessage(`{"updated":true}`), nil
}

func TestMCPClientsKeepTheUsersAuthorityWhetherChattingOrAutomatic(t *testing.T) {
	actor := principal.Principal{Type: principal.PrincipalAgent, ID: "agent:external", OnBehalfOf: ids.NewV7(), UserID: ids.NewV7(), PassportID: ids.NewV7(), ConnectionID: ids.NewV7(), SeatType: principal.SeatFull, Permissions: everyRecordedGrant(), Scopes: principal.NewScopeSet(principal.ScopeSend)}
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), actor)
	tool := &userAuthorityTool{spec: mcp.ToolSpec{Name: "user_change", Title: "Change a record", Version: testToolVersion, Description: "Change the user record.", RequiredScope: principal.ScopeSend, Tier: mcp.TierAutoExecute, InputSchema: json.RawMessage(`{"type":"object"}`)}, seen: make(chan context.Context, 3)}
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(tool)
	handler := NewHTTPHandler(registry, func(*http.Request) (context.Context, error) { return ctx, nil }, func(*http.Request) string { return "" }, "margince-crm", "test", discardLog())
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, client := range []string{"Claude Desktop", "n8n scheduled workflow", "curl/automatic"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"user_change","arguments":{},"_meta":{"io.modelcontextprotocol/clientInfo":{"name":%q,"version":"1.0"}}}}`, client)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", client)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if err != nil || !strings.Contains(string(body), `updated`) {
			t.Fatalf("%s: %s (%v)", client, body, err)
		}
		seen := <-tool.seen
		got, ok := principal.Actor(seen)
		if !ok || got.ID != actor.ID || got.OnBehalfOf != actor.OnBehalfOf {
			t.Fatalf("%s changed the acting user: %+v", client, got)
		}
		if _, set := principal.AgentRunID(seen); set || principal.Unattended(seen) {
			t.Fatalf("%s was classified as a built-in scheduled run", client)
		}
	}
}
