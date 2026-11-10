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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// installationPolicy answers as an installation reads it: Admin is unpaced.
type installationPolicy struct{}

func (installationPolicy) Policy(context.Context) (agentvolume.Policy, error) {
	return agentvolume.Policy{Unpaced: identity.UnpacedRoles}, nil
}

// pacedSurface is /mcp composed as routes.go composes it, where a bearer names
// its human's roles and a bearer not in roles is unknown.
type pacedSurface struct {
	handler http.Handler
	clock   *stepClock
	roles   map[string][]string
	lookups int
}

func newPacedSurface(roles map[string][]string) *pacedSurface {
	s := &pacedSurface{clock: newStepClock(), roles: roles}
	lim := newMCPLimitersWithClock(s.clock.now)
	meter := agentvolume.NewWithClock(nil, agentvolume.Limits{}, agentvolume.DefaultWindow, s.clock.now).WithPolicySource(installationPolicy{})
	workspace := ids.NewV7()
	authenticate := func(r *http.Request) (context.Context, error) {
		s.lookups++
		held, known := s.roles[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if !known {
			return nil, apperrors.ErrNotFound
		}
		return principal.WithActor(principal.WithWorkspaceID(r.Context(), workspace), principal.Principal{
			Type: principal.PrincipalAgent, Permissions: principal.Permissions{RoleKeys: held},
		}), nil
	}
	s.handler = mcpEdge(agents.NewHTTPHandler(NewRegistry(nil, SendPath{}), mcpPaceAfterAuth(authenticate, lim.perPassport, meter),
		func(*http.Request) string { return "Bearer" }, "margince-crm", "paced", slog.New(slog.NewTextHandler(io.Discard, nil))),
		lim, "https://crm.example.com")
	return s
}

func (s *pacedSurface) call(bearer string) *httptest.ResponseRecorder {
	r := mcpRequest(http.MethodPost, bearer, "160.79.104.11")
	r.Body = io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, r)
	return rec
}

func (s *pacedSurface) spend(t *testing.T, bearer string, calls int) {
	t.Helper()
	for i := 1; i <= calls; i++ {
		if s.call(bearer).Code == http.StatusTooManyRequests {
			t.Fatalf("%s's call %d was refused for pacing", bearer, i)
		}
	}
}

// A User's passport is paced at the ceiling, with the pacing and the wait
// named, and is refused there before any passport lookup.
func TestAUsersPassportIsPacedAtTheCeiling(t *testing.T) {
	s := newPacedSurface(map[string][]string{"rep": {"rep"}, "colleague": {"rep"}})
	s.spend(t, "rep", mcpCallsPerMinute)
	refused := s.call("rep")
	if refused.Code != http.StatusTooManyRequests || refused.Header().Get("Retry-After") != "60" ||
		!strings.Contains(refused.Body.String(), `"kind":"pacing"`) || !strings.Contains(refused.Body.String(), `"ceiling":240`) {
		t.Fatalf("call %d → %d %s, want 429 naming pacing at 240 with a 60-second wait", mcpCallsPerMinute+1, refused.Code, refused.Body)
	}
	s.spend(t, "colleague", mcpCallsPerMinute)
	if before := s.lookups; s.call("rep").Code != http.StatusTooManyRequests || s.lookups != before {
		t.Error("a passport over its pacing was looked up before being refused")
	}
}

// An Admin is never paced, and no caller's pacing reaches the next. Roles are
// read on every call. Anonymous, refused or served requests spend nothing.
func TestPacingNeverCarriesOverBetweenCallers(t *testing.T) {
	s := newPacedSurface(map[string][]string{"admin": {"admin"}, "rep": {"rep"}, "demoted": {"admin"}})
	for range 10 {
		if s.call("").Code != http.StatusUnauthorized || s.call("later").Code != http.StatusUnauthorized {
			t.Fatal("an anonymous or unknown caller was not refused with 401")
		}
	}
	s.spend(t, "admin", 500)
	s.spend(t, "demoted", 500)
	s.spend(t, "rep", mcpCallsPerMinute)
	if s.call("rep").Code != http.StatusTooManyRequests {
		t.Fatal("the User's passport was not paced after its own calls")
	}
	s.spend(t, "admin", 500)
	s.roles["later"], s.roles["demoted"] = []string{"rep"}, []string{"rep"}
	s.spend(t, "later", mcpCallsPerMinute)
	s.spend(t, "demoted", mcpCallsPerMinute)
	if s.call("demoted").Code != http.StatusTooManyRequests || s.call("").Code != http.StatusUnauthorized {
		t.Error("a passport whose human lost Admin kept no pacing, or served calls spent the pre-auth ceiling")
	}
}
