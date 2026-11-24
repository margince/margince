// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// A call that waits for its own user, confirmed in the MCP conversation instead
// of the approval inbox (agents/confirmcode.go). Driven over the composed /mcp
// door with a real OAuth connection, so the wiring is production's.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/platform/replayguard"
)

type confirmEnv struct {
	*oauthEnv
	project string
}

func setupConfirm(t *testing.T) *confirmEnv {
	t.Helper()
	o := setupOAuthWith(t,
		compose.WithMCPInputStateKey(strings.Repeat("k", 32)),
		compose.WithMCPConfirmGuard(replayguard.New(redistest.Client(t))))
	var company, project struct {
		ID string `json:"id"`
	}
	if status := o.Call(t, "POST", "/v1/companies", integration.AnyMap{"source": "manual", "display_name": "Confirm Account"}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}
	if status := o.Call(t, "POST", "/v1/projects", integration.AnyMap{"name": "Confirm Engagement", "company_id": company.ID, "source": "manual"}, nil, &project); status != http.StatusCreated {
		t.Fatalf("create project → %d", status)
	}
	return &confirmEnv{oauthEnv: o, project: project.ID}
}

// call answers one tools/call's result over the composed door.
func (e *confirmEnv) call(t *testing.T, bearer, tool, args string) map[string]any {
	t.Helper()
	resp := mcpRaw(t, e.AppEnv, http.MethodPost, "/mcp",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+tool+`","arguments":`+args+`}}`,
		map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + bearer})
	var rpc struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &rpc); err != nil || rpc.Result == nil {
		t.Fatalf("tools/call %s → %d %s", tool, resp.StatusCode, resp.Body)
	}
	return rpc.Result
}

// fileOnProject logs a note and answers the arguments that file it under the
// project.
func (e *confirmEnv) fileOnProject(t *testing.T, bearer string) (args, activity string) {
	t.Helper()
	logged := e.call(t, bearer, "log_activity", `{"kind":"note","body":"file me under the project"}`)
	content, _ := logged["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("log_activity → %v, want its answer", logged)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("reading the logged activity: %v (%s)", err, text)
	}
	return `{"activity_id":"` + envelope.Data.ID + `","entity_type":"project","entity_id":"` + e.project + `"}`, envelope.Data.ID
}

func (e *confirmEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Filing under a project waits for its own user. On a connected client, round 1
// files nothing and stages nothing. The user's yes then files it once, the audit
// row names the code, and the code does not work twice.
func TestAProjectFilingIsConfirmedInTheConversationNotTheInbox(t *testing.T) {
	e := setupConfirm(t)
	access, _ := e.connect(t)
	args, activity := e.fileOnProject(t, access)
	filed := func() int {
		return e.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND project_id = $2`, activity, e.project)
	}
	staged := func() int { return e.count(t, `SELECT count(*) FROM approval`) }

	asked := e.call(t, access, "relink_activity", args)
	meta, _ := asked["_meta"].(map[string]any)
	fields, _ := meta["margince/confirm"].(map[string]any)
	code, _ := fields["confirm_code"].(string)
	if code == "" {
		t.Fatalf("round 1 → %v, want a preview and a confirm_code", asked)
	}
	if filed() != 0 || staged() != 0 {
		t.Fatalf("round 1 filed %d and staged %d, want neither", filed(), staged())
	}

	withCode := strings.TrimSuffix(args, "}") + `,"confirm_code":"` + code + `"}`
	if done := e.call(t, access, "relink_activity", withCode); done["isError"] == true {
		t.Fatalf("round 2 with the user's yes → %v", done)
	}
	if filed() != 1 || staged() != 0 {
		t.Fatalf("after the yes: filed %d, staged %d; want filed once and nothing in the inbox", filed(), staged())
	}
	if e.count(t, `SELECT count(*) FROM audit_log WHERE evidence ->> $1 IS NOT NULL`, storekit.EvidenceKeyConfirmedInConversation) == 0 {
		t.Error("the filing's audit rows do not name the code that confirmed it")
	}
	if again := e.call(t, access, "relink_activity", withCode); again["isError"] != true {
		t.Errorf("the same code a second time → %v, want it refused", again)
	}
}

// A passport minted for a script has no connection, so nobody is in a
// conversation to say yes. Its call waits in the inbox and gets no code.
func TestAPassportWithoutAConnectionStillWaitsInTheInbox(t *testing.T) {
	e := setupConfirm(t)
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", integration.AnyMap{"label": "script", "scopes": []string{"read", "write"}}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("minting a passport → %d", status)
	}
	args, _ := e.fileOnProject(t, minted.Token)
	asked := e.call(t, minted.Token, "relink_activity", args)
	if meta, _ := asked["_meta"].(map[string]any); meta["margince/confirm"] != nil {
		t.Fatalf("a script's call → %v, want no confirm_code", asked)
	}
	if e.count(t, `SELECT count(*) FROM approval`) != 1 {
		t.Error("a script's waiting call must stage into the approval inbox")
	}
}
