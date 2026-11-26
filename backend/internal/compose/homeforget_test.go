// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAWriteEndsTheCachedHomeOfEachReaderItWasMadeForOnce(t *testing.T) {
	human, other := ids.NewV7(), ids.NewV7()
	cases := map[string]struct {
		method string
		actor  principal.Principal
		want   []ids.UUID
	}{
		"a human's write":                    {"POST", principal.Principal{Type: principal.PrincipalHuman, UserID: human}, []ids.UUID{human}},
		"an agent acting as its human":       {"PUT", principal.Principal{Type: principal.PrincipalAgent, UserID: human, OnBehalfOf: human}, []ids.UUID{human}},
		"an agent seat acting for its human": {"PATCH", principal.Principal{Type: principal.PrincipalAgent, UserID: other, OnBehalfOf: human}, []ids.UUID{other, human}},
		"a read":                             {"GET", principal.Principal{Type: principal.PrincipalHuman, UserID: human}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "/v1/contacts", nil)
			r = r.WithContext(principal.WithActor(r.Context(), c.actor))
			if got := writersOf(r); !slices.Equal(got, c.want) {
				t.Errorf("writers %v, want %v", got, c.want)
			}
		})
	}
}

func requestOn(route string) *http.Request {
	method, pattern, _ := strings.Cut(route, " ")
	rctx, r := chi.NewRouteContext(), httptest.NewRequest(method, pattern, nil)
	rctx.RoutePatterns = []string{pattern}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestARouteThatOnlyReadsNeverMarksHomeBehind(t *testing.T) { // census over agentPolicies: a new read route is covered
	checked := 0
	for route, pol := range agentPolicies {
		method, _, _ := strings.Cut(route, " ")
		if !mutatingMethod(method) || pol.Scope != scopeRead {
			continue
		}
		checked++
		if mayChangeHome(requestOn(route)) {
			t.Errorf("%s only reads, and marks Home behind a write", route)
		}
	}
	if !mayChangeHome(requestOn("PATCH /v1/contacts/{id}")) || checked == 0 {
		t.Fatalf("a contact update must mark Home behind, and the census found %d read routes to check", checked)
	}
}

func TestEveryWriteHomeIgnoresIsAWriteRoute(t *testing.T) {
	for route := range homeUnchangedBy {
		if pol, ok := agentPolicies[route]; !ok || pol.Scope == scopeRead {
			t.Errorf("%s is listed but is not a write route", route)
		}
	}
}
