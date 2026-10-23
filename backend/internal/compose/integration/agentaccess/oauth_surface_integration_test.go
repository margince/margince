// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// What the minted Bearer is once the handshake is done: a passport with real
// authority on the governed surfaces, and nothing more. A call that still waits
// stages an approval it redeems once, by the approval's id. Its authority dies on the
// next call after revocation, asserted on the hosted MCP transport.

func TestAnApprovedCallRedeemsOnceByItsApprovalID(t *testing.T) {
	o := setupOAuth(t)

	code := o.authorize(t, url.Values{"scope": {"read enrich"}})
	_, body := o.exchange(t, url.Values{"code": {code}})
	agentBearer := map[string]string{"Authorization": "Bearer " + body["access_token"].(string)}

	// Reading a company's site still asks a human first, even on the passport
	// of the colleague who connected: the model picks the address it reads.
	var company struct {
		ID string `json:"id"`
	}
	if status := o.Call(t, "POST", "/v1/companies", integration.AnyMap{
		"source": "manual", "display_name": "Redeem Once GmbH",
		"domains": []integration.AnyMap{{"domain": "example.com", "is_primary": true}},
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("company create → %d", status)
	}
	enrich := "/v1/companies/" + company.ID + "/enrich"
	var problem struct {
		Detail string `json:"detail"`
	}
	if status := o.Call(t, "POST", enrich, nil, agentBearer, &problem); status != http.StatusForbidden {
		t.Fatalf("agent enrich → %d, want staged 403", status)
	}
	approvalID := integration.ExtractStagedApprovalID(t, problem.Detail)
	if status := o.Call(t, "POST", "/v1/approvals/"+approvalID+"/approve", integration.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("approve → %d", status)
	}

	// The approved call passes the gate. What enrich then does is its own
	// business, so only a refusal from the gate would fail this.
	redeeming := map[string]string{"Authorization": agentBearer["Authorization"], "X-Approval-Token": approvalID}
	var redeemed struct {
		Code string `json:"code"`
	}
	status := o.Call(t, "POST", enrich, nil, redeeming, &redeemed)
	if redeemed.Code == "approval_required" || redeemed.Code == "approval_token_invalid" {
		t.Fatalf("the approved call repeated with its approval id → %d %q, want it past the gate", status, redeemed.Code)
	}
	var replay struct {
		Code string `json:"code"`
	}
	if status := o.Call(t, "POST", enrich, nil, redeeming, &replay); status != http.StatusForbidden || replay.Code != "approval_token_invalid" {
		t.Errorf("a second redemption → %d %q, want 403 approval_token_invalid", status, replay.Code)
	}
}

func TestHostedMCPTransportSharesTheGovernedSurface(t *testing.T) {
	o := setupOAuth(t)
	code := o.authorize(t, nil)
	_, body := o.exchange(t, url.Values{"code": {code}})
	token := body["access_token"].(string)

	pool, err := testdb.OwnPool(context.Background(), apptest.AppDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	authSvc := identity.NewService(pool)
	registry := compose.NewRegistry(pool, compose.SendPath{})
	authenticate := func(r *http.Request) (context.Context, error) {
		wsID, err := authSvc.InstallationWorkspace(r.Context())
		if err != nil {
			return nil, err
		}
		ctx := principal.WithWorkspaceID(r.Context(), wsID.UUID)
		agent, err := authSvc.AuthenticateAgent(ctx, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil {
			return nil, err
		}
		return principal.WithCorrelationID(principal.WithActor(ctx, agent.Principal()), ids.NewV7()), nil
	}
	hosted := httptest.NewServer(agents.NewHTTPHandler(registry, authenticate,
		agents.ResourceMetadataChallenge("https://crm.example.test"), "margince-crm", "test",
		slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(hosted.Close)

	rpc := func(bearer, payload string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, hosted.URL, strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer apptest.CloseBody(t, resp)
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	status, out := rpc(token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if status != http.StatusOK || !strings.Contains(out, `"find_records"`) {
		t.Fatalf("hosted tools/list → %d %s", status, out)
	}
	status, out = rpc(token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_record","arguments":{"record_type":"contact","fields":{"full_name":"Hosted Agent Contact"}}}}`)
	if status != http.StatusOK || !strings.Contains(out, "Hosted Agent Contact") {
		t.Fatalf("hosted tools/call → %d %s", status, out)
	}

	// Revocation binds between two calls: kill the passport via the
	// session surface, the next hosted call answers 401 + RFC 9728.
	var passportID string
	if err := o.Owner.QueryRow(context.Background(),
		`SELECT id FROM passport WHERE token_hash = $1`, sha256Hex(token)).Scan(&passportID); err != nil {
		t.Fatal(err)
	}
	if status := o.Call(t, "DELETE", "/v1/passports/"+passportID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("revoke → %d", status)
	}
	req, _ := http.NewRequest(http.MethodPost, hosted.URL, strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(resp.Header.Get("WWW-Authenticate"), "oauth-protected-resource") {
		t.Fatalf("revoked bearer → %d %q, want 401 + RFC 9728 pointer", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}
