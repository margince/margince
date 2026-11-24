// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A call that waits for its own user, confirmed in the MCP conversation instead
// of the approval inbox (agents/confirmcode.go). Driven through the real MCP
// handler on a real passport, so the Dispatcher arms it as production does.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/platform/replayguard"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mcpDoor serves the registry over MCP HTTP with chat confirmation armed, and
// answers one tools/call's result.
func (q *queueEnv) mcpDoor(t *testing.T, token string) func(tool, args string) map[string]any {
	t.Helper()
	authenticate := func(r *http.Request) (context.Context, error) {
		ws, err := q.auth.InstallationWorkspace(r.Context())
		if err != nil {
			return nil, err
		}
		ctx := principal.WithWorkspaceID(r.Context(), ws.UUID)
		agent, err := q.auth.AuthenticateAgent(ctx, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil {
			return nil, err
		}
		return principal.WithCorrelationID(principal.WithActor(ctx, agent.Principal()), ids.NewV7()), nil
	}
	door := httptest.NewServer(agents.NewHTTPHandler(q.registry, authenticate,
		agents.ResourceMetadataChallenge("https://crm.example.test"), "margince-crm", "test",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		agents.WithInputStateKey(strings.Repeat("k", 32)),
		agents.WithConfirmGuard(replayguard.New(redistest.Client(t)))))
	t.Cleanup(door.Close)
	return func(tool, args string) map[string]any {
		t.Helper()
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`
		req, _ := http.NewRequest(http.MethodPost, door.URL, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer apptest.CloseBody(t, resp)
		var rpc struct {
			Result map[string]any `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil || rpc.Result == nil {
			t.Fatalf("tools/call %s: %v", tool, err)
		}
		return rpc.Result
	}
}

// Filing under a project waits for its own user. In the conversation, round 1
// files nothing and stages nothing. The user's yes then files it once, and the
// audit row names the code that confirmed it.
func TestAProjectFilingIsConfirmedInTheConversationNotTheInbox(t *testing.T) {
	q := setupQueue(t)
	token := q.mintPassport(t, "confirming agent", "read", "write")
	call := q.mcpDoor(t, token)

	var company, project struct {
		ID string `json:"id"`
	}
	if status := q.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": "Confirm Account"}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}
	if status := q.Call(t, "POST", "/v1/projects", AnyMap{"name": "Confirm Engagement", "company_id": company.ID, "source": "manual"}, nil, &project); status != http.StatusCreated {
		t.Fatalf("create project → %d", status)
	}
	logged := call("log_activity", `{"kind":"note","body":"file me under the project"}`)
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	text := logged["content"].([]any)[0].(map[string]any)["text"].(string)
	if err := json.Unmarshal([]byte(text), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("reading the logged activity: %v (%s)", err, text)
	}
	args := `{"activity_id":"` + envelope.Data.ID + `","entity_type":"project","entity_id":"` + project.ID + `"}`
	owner := OwnerConn(t)
	filed := func() int {
		var n int
		if err := owner.QueryRow(context.Background(),
			`SELECT count(*) FROM activity_link WHERE activity_id = $1 AND project_id = $2`, envelope.Data.ID, project.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	approvals := func() int {
		var n int
		if err := owner.QueryRow(context.Background(), `SELECT count(*) FROM approval`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	asked := call("relink_activity", args)
	fields, _ := asked["_meta"].(map[string]any)["margince/confirm"].(map[string]any)
	code, _ := fields["confirm_code"].(string)
	if code == "" {
		t.Fatalf("round 1 → %v, want a preview and a confirm_code", asked)
	}
	if filed() != 0 || approvals() != 0 {
		t.Fatalf("round 1 filed %d and staged %d, want neither", filed(), approvals())
	}

	withCode := strings.TrimSuffix(args, "}") + `,"confirm_code":"` + code + `"}`
	if done := call("relink_activity", withCode); done["isError"] == true {
		t.Fatalf("round 2 with the user's yes → %v", done)
	}
	if filed() != 1 || approvals() != 0 {
		t.Fatalf("after the yes: filed %d, staged %d; want filed once and nothing in the inbox", filed(), approvals())
	}
	var confirmedRows int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE evidence ->> $1 IS NOT NULL`, storekit.EvidenceKeyConfirmedInConversation).Scan(&confirmedRows); err != nil {
		t.Fatal(err)
	}
	if confirmedRows == 0 {
		t.Error("the filing's audit rows do not name the code that confirmed it")
	}
	if again := call("relink_activity", withCode); again["isError"] != true {
		t.Errorf("the same code a second time → %v, want it refused", again)
	}
}
