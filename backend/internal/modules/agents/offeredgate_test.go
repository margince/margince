// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// callOverMCP posts one tools/call as an agent holding every grant, and answers
// the body the client reads.
func callOverMCP(t *testing.T, name string) string {
	t.Helper()
	h := NewHTTPHandler(fullRegistry(t), func(*http.Request) (context.Context, error) { return agentCtx(), nil },
		func(*http.Request) string { return "" }, "margince-crm", "test", slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the response: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	return string(body)
}

// A client calls only what tools/list offers it. A name the listing leaves out
// answers as a name that does not exist, whether a router's target or a
// superseded tool. The refusal tells a caller nothing the listing did not.
func TestAToolTheListingLeavesOutIsRefusedAtTheDoor(t *testing.T) {
	for _, hidden := range []string{"advance_deal", "relink_thread", "list_records"} {
		if answer := callOverMCP(t, hidden); !strings.Contains(answer, "unknown tool "+hidden) {
			t.Errorf("a call to the hidden %s was served: %s", hidden, answer)
		}
	}
	if answer := callOverMCP(t, "change_tags"); strings.Contains(answer, "unknown tool") {
		t.Errorf("the router the listing offers was refused at the door: %s", answer)
	}
	// A listed tool this caller's grants trim from its list still passes the
	// door, and its gate gives the real answer.
	if answer := callOverMCP(t, "read_record"); strings.Contains(answer, "unknown tool") {
		t.Errorf("a listed tool was refused at the door instead of by its gate: %s", answer)
	}
}

// unavailableVerbs is a feature policy that switches off one verb a router
// fronts and one tool that stands alone.
type unavailableVerbs struct{}

func (unavailableVerbs) Admits(verb, _ string) bool {
	return verb != "archive_tag" && verb != "read_forecast"
}
func (unavailableVerbs) Context(ctx context.Context) context.Context { return ctx }

// The door holds surface membership and nothing else, under each wiring and
// each kind of caller. It passes every name tools/list shows, and every listed
// tool the caller's list trims. It refuses every hidden tool the gate does not
// admit, and follows the gate for a Gated one.
func TestTheDoorAdmitsExactlyThePublicSurface(t *testing.T) {
	registries := map[string]*Registry{
		"plain":          fullRegistry(t),
		"gate admits":    fullRegistry(t, WithToolGate(func(context.Context) bool { return true })),
		"some verbs off": fullRegistry(t, WithCallAvailability(unavailableVerbs{})),
	}
	agent := func(scope principal.Scope, grants principal.Permissions, areas principal.WorkAreas) context.Context {
		return principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:census", OnBehalfOf: ids.NewV7(), SeatType: principal.SeatFull,
			Scopes: principal.NewScopeSet(scope), Permissions: grants, Areas: areas,
		})
	}
	callers := map[string]context.Context{
		"every grant":   agentCtx(),
		"read scope":    agent(principal.ScopeRead, everyRecordedGrant(), principal.WorkAreas{}),
		"no grants":     agent(principal.ScopeWrite, principal.Permissions{}, principal.WorkAreas{}),
		"one work area": agent(principal.ScopeWrite, everyRecordedGrant(), principal.WorkAreas{Limited: true, Chosen: []string{"business"}}),
		"no actor":      context.Background(),
	}
	for wiring, r := range registries {
		specs := r.Specs()
		if len(specs) == 0 {
			t.Fatalf("%s: no tool is declared, so nothing was checked", wiring)
		}
		for who, ctx := range callers {
			offered := r.Offered(ctx)
			for _, spec := range specs {
				listed := slices.ContainsFunc(offered, func(o mcp.ToolSpec) bool { return o.Name == spec.Name })
				want := !spec.Unlisted || (spec.Gated && listed)
				if got := r.OnSurface(ctx, spec.Name); got != want || (listed && !got) {
					t.Errorf("%s, %s: %s (unlisted %v, gated %v, listed %v) is on the surface %v, want %v",
						wiring, who, spec.Name, spec.Unlisted, spec.Gated, listed, got, want)
				}
			}
		}
	}
}

// servingUnofferedNames lets one test call a verb by its own name, as a test of
// that verb's arguments does. The test above holds the door.
func servingUnofferedNames(t *testing.T) {
	t.Helper()
	previous := servesUnoffered.Swap(true)
	t.Cleanup(func() { servesUnoffered.Store(previous) })
}
