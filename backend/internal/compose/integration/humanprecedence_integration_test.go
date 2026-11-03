// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An agent edit applies in the one call, whoever last wrote the field. The audit
// trail and Undo put a wrong edit back, so nothing waits for approval. An agent
// on a colleague's passport and automatic work with no passport both apply it.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAgentOnAColleaguesPassportOverwritesAHumanTypedField(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	contactID, agentToken := stageContactAndAgent(t, e, "Greta Human", "precedence agent")
	bearer := map[string]string{"Authorization": "Bearer " + agentToken}

	var patched struct {
		FullName       string          `json:"full_name"`
		Title          string          `json:"title"`
		StagedApproval json.RawMessage `json:"staged_approval"`
	}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+contactID,
		AnyMap{"full_name": "Greta Machine", "title": "CTO"}, bearer, &patched); status != 200 {
		t.Fatalf("agent overwrite on a colleague's passport → %d, want 200", status)
	}
	if patched.FullName != "Greta Machine" || patched.Title != "CTO" || len(patched.StagedApproval) != 0 {
		t.Fatalf("patched = %+v, want the whole patch applied and nothing staged", patched)
	}
	if n := countApprovals(t, e); n != 0 {
		t.Errorf("%d approvals staged, want none", n)
	}
}

// A differently-cased field key is refused, not applied. Every record request
// decoder matches its known fields by exact key and refuses any other key that
// is no cf_ key. The patch answers 422 and the stored value stands. Lead is
// exercised here because it was the last object to take custom fields.
func TestEndToEnd_caseVariantKeyIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var lead struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/leads", AnyMap{"full_name": "Otto Human"}, nil, &lead); status != 201 {
		t.Fatalf("human create lead → %d", status)
	}
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "variant agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != 201 {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	// The case-variant key binds to no known field (exact-key decode) and is no
	// cf_ key, so it is refused by name and nothing is written.
	if status := e.Call(t, "PATCH", "/v1/leads/"+lead.ID, AnyMap{"FULL_NAME": "Otto Machine"}, bearer, nil); status != 422 {
		t.Fatalf("case-variant patch → %d, want 422 (refused, not written)", status)
	}
	var patched struct {
		FullName string `json:"full_name"`
	}
	if status := e.Call(t, "GET", "/v1/leads/"+lead.ID, nil, nil, &patched); status != 200 {
		t.Fatalf("read the lead → %d", status)
	}
	if patched.FullName != "Otto Human" {
		t.Fatalf("case-variant key must not overwrite the human value: got %q", patched.FullName)
	}
	// And it survives a fresh read — the human-owned value was never touched.
	var current struct {
		FullName string `json:"full_name"`
	}
	if status := e.Call(t, "GET", "/v1/leads/"+lead.ID, nil, bearer, &current); status != 200 || current.FullName != "Otto Human" {
		t.Fatalf("human value must survive the case-variant attempt: %d %q", status, current.FullName)
	}
}

// stageContactAndAgent is the scenario floor the split tests share: a
// HUMAN creates the contact (full_name becomes human-owned per the audit
// trail) and mints a read+write passport for the acting agent.
func stageContactAndAgent(t *testing.T, e *apptest.AppEnv, fullName, label string) (contactID, agentToken string) {
	t.Helper()
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": fullName}, nil, &contact); status != 201 {
		t.Fatalf("human create → %d", status)
	}
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": label, "scopes": []string{"read", "write"},
	}, nil, &minted); status != 201 {
		t.Fatalf("issue passport → %d", status)
	}
	return contact.ID, minted.Token
}

// mcpAgentInvoker builds the governed tool registry over the shared
// database and returns an Invoke closure that re-authenticates the
// passport per call, exactly as the stdio/hosted transports do.
func mcpAgentInvoker(t *testing.T, agentToken string) func(tool, args string) (json.RawMessage, error) {
	t.Helper()
	pool, err := testdb.OwnPool(context.Background(), apptest.AppDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	registry := compose.NewRegistry(pool, compose.SendPath{})
	authSvc := identity.NewService(pool)
	return func(tool, args string) (json.RawMessage, error) {
		wsID, err := authSvc.InstallationWorkspace(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ctx := principal.WithWorkspaceID(context.Background(), wsID.UUID)
		agent, err := authSvc.AuthenticateAgent(ctx, agentToken)
		if err != nil {
			t.Fatal(err)
		}
		ctx = principal.WithCorrelationID(principal.WithActor(ctx, agent.Principal()), ids.NewV7())
		return registry.Invoke(ctx, tool, json.RawMessage(args))
	}
}

// contactNameAndTitle reads a contact back through the governed tool
// surface and returns the two fields the split scenarios assert on.
func contactNameAndTitle(t *testing.T, invoke func(tool, args string) (json.RawMessage, error), contactID string) (fullName, title string) {
	t.Helper()
	out, err := invoke("read_record", fmt.Sprintf(`{"record_type":"contact","id":%q}`, contactID))
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		Fields struct {
			FullName string `json:"full_name"`
			Title    string `json:"title"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(ToolPayload(t, out), &rec); err != nil {
		t.Fatal(err)
	}
	return rec.Fields.FullName, rec.Fields.Title
}

// Automatic work with no passport applies the whole patch, including a field a
// human typed. Nothing is staged for it.
func TestAutomaticWorkOverwritesAHumanTypedFieldWithoutApproval(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	contactID, _ := stageContactAndAgent(t, e, "Mona Human", "automatic edit agent")
	invoke := automaticInvoker(t, e)

	out, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"contact","id":%q,"fields":{"full_name":"Mona Machine","title":"VP Engineering"}}`, contactID,
	))
	if err != nil {
		t.Fatalf("a patch touching a human-typed name → %v, want it applied", err)
	}
	if strings.Contains(string(out), "staged_approval") {
		t.Fatalf("the patch staged something: %s", out)
	}
	fullName, title := contactNameAndTitle(t, invoke, contactID)
	if fullName != "Mona Machine" || title != "VP Engineering" {
		t.Errorf("the contact reads %q / %q, want the whole patch applied", fullName, title)
	}
	if n := countApprovals(t, e); n != 0 {
		t.Errorf("automatic work left %d approvals, want none", n)
	}
}

// automaticWork is an agent with no passport behind it, acting for the
// signed-in colleague. It is refused a confirm-first call.
func automaticWork(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	var ws ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM workspace LIMIT 1`).Scan(&ws); err != nil {
		t.Fatalf("reading the workspace: %v", err)
	}
	colleague, err := ids.Parse(sessionUserID(t, e))
	if err != nil {
		t.Fatalf("reading the signed-in colleague: %v", err)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:automatic", OnBehalfOf: colleague,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
	})
}

// automaticInvoker is mcpAgentInvoker for automatic work: the composed tool
// registry, called in-process by an agent with no passport.
func automaticInvoker(t *testing.T, e *apptest.AppEnv) func(tool, args string) (json.RawMessage, error) {
	t.Helper()
	pool, err := testdb.OwnPool(context.Background(), apptest.AppDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	registry := compose.NewRegistry(pool, compose.SendPath{})
	return func(tool, args string) (json.RawMessage, error) {
		return registry.Invoke(automaticWork(t, e), tool, json.RawMessage(args))
	}
}

// countApprovals is how many approvals the workspace holds, in any state.
func countApprovals(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM approval`).Scan(&n); err != nil {
		t.Fatalf("counting approvals: %v", err)
	}
	return n
}

// isPassportlessStagingRefusal reports the staging engine's refusal of an agent
// with no passport. The engine answers it with a plain error, so its words are
// matched.
func isPassportlessStagingRefusal(err error) bool {
	return err != nil && strings.Contains(err.Error(), "an agent stages on its passport")
}
