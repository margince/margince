// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type installationPolicy struct{}

func (installationPolicy) Policy(context.Context) (agentvolume.Policy, error) {
	return agentvolume.Policy{Unpaced: identity.UnpacedRoles}, nil
}

// /mcp as routes.go composes it, where a bearer names its human's roles. An
// Admin is never paced; a User is, at the ceiling and before any lookup. Roles
// are read on every call, and anonymous, refused or served calls spend nothing.
func TestOnlyAnAdminsPassportGoesUnpacedOnMCP(t *testing.T) {
	roles := map[string][]string{"admin": {"admin"}, "rep": {"rep"}, "demoted": {"admin"}}
	lim, lookups := newMCPLimitersWithClock(newStepClock().now), 0
	meter := agentvolume.New(nil, agentvolume.Limits{}, agentvolume.DefaultWindow).WithPolicySource(installationPolicy{})
	authenticate := func(r *http.Request) (context.Context, error) {
		lookups++
		held, known := roles[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if !known {
			return nil, apperrors.ErrNotFound
		}
		agent := principal.Principal{Type: principal.PrincipalAgent, Permissions: principal.Permissions{RoleKeys: held}}
		return principal.WithActor(principal.WithWorkspaceID(r.Context(), [16]byte{1}), agent), nil
	}
	edge := mcpEdge(agents.NewHTTPHandler(NewRegistry(nil, SendPath{}), mcpPaceAfterAuth(authenticate, lim.perPassport, meter),
		func(*http.Request) string { return "Bearer" }, "margince-crm", "paced", slog.New(slog.NewTextHandler(io.Discard, nil))),
		lim, "https://crm.example.com")
	call := func(bearer string, times int) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		for i := 1; i <= times; i++ {
			r := mcpRequest(http.MethodPost, bearer, "160.79.104.11")
			r.Body, rec = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)), httptest.NewRecorder()
			if edge.ServeHTTP(rec, r); rec.Code == http.StatusTooManyRequests && i < times {
				t.Fatalf("%s's call %d was refused for pacing", bearer, i)
			}
		}
		return rec
	}
	if call("", 10).Code != http.StatusUnauthorized || call("later", 10).Code != http.StatusUnauthorized {
		t.Fatal("an anonymous or unknown caller was not refused with 401")
	}
	call("admin", 1000)
	call("demoted", 500)
	refused := call("rep", mcpCallsPerMinute+1)
	if body := refused.Body.String(); refused.Code != http.StatusTooManyRequests || refused.Header().Get("Retry-After") != "60" ||
		!strings.Contains(body, `"kind":"pacing"`) || !strings.Contains(body, `"ceiling":240`) {
		t.Fatalf("the User's call 241 → %d %s, want 429 naming pacing at 240 with a 60-second wait", refused.Code, body)
	}
	if before := lookups; call("rep", 1).Code != http.StatusTooManyRequests || lookups != before {
		t.Error("a passport over its pacing was looked up before being refused")
	}
	roles["later"], roles["demoted"] = []string{"rep"}, []string{"rep"}
	call("admin", 500)
	call("later", mcpCallsPerMinute)
	if call("demoted", mcpCallsPerMinute+1).Code != http.StatusTooManyRequests || call("", 1).Code != http.StatusUnauthorized {
		t.Error("a passport whose human lost Admin kept no pacing, or served calls spent the pre-auth ceiling")
	}
}
