// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// The members the contract marks human-only on the signal edit are refused in
// an agent's body, and everything else the route takes passes on unread.
func TestAnAgentBodySettingAHumanOnlyMemberIsRefused(t *testing.T) {
	if got := routeVerbShapes["PATCH /v1/signals/{id}"].HumanOnly; len(got) == 0 {
		t.Fatalf("the generated table marks no human-only member on the signal edit")
	}
	for body, refused := range map[string]bool{
		`{"kind":"risk"}`:           true,
		`{"evidence":[]}`:           true,
		`{"status":"acknowledged"}`: false,
		`not json`:                  false,
	} {
		rctx := chi.NewRouteContext()
		rctx.RoutePatterns = []string{"/v1/signals/{id}"}
		r := httptest.NewRequest(http.MethodPatch, "/v1/signals/x", strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		if got := refusedHumanOnlyMember(w, r); got != refused {
			t.Errorf("%s: refused = %v, want %v", body, got, refused)
			continue
		}
		if refused && w.Code != http.StatusForbidden {
			t.Errorf("%s: answered %d, want 403", body, w.Code)
		}
		rest, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !refused && string(rest) != body {
			t.Errorf("%s: the handler would read %q, want the body unchanged", body, rest)
		}
	}
}
