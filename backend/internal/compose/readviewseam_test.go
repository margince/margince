// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A view past the answer cap is refused by name, and one under it passes whole.
func TestAViewPastTheAnswerCapIsRefused(t *testing.T) {
	view := agents.ReadView{Name: "export", Op: "export", Pattern: "/v1/export"}
	for _, tc := range []struct {
		name    string
		size    int
		refused bool
	}{
		{"under the cap", maxViewAnswerBytes - 64, false},
		{"over the cap", maxViewAnswerBytes + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/csv")
				for written := 0; written < tc.size; written += 1024 {
					if _, err := w.Write(bytes.Repeat([]byte("a"), min(1024, tc.size-written))); err != nil {
						t.Errorf("writing the view: %v", err)
					}
				}
			})}
			answer, err := readViewThrough(door)(context.Background(), view, nil, nil)
			var detailed *httperr.DetailedError
			switch {
			case tc.refused && !errors.As(err, &detailed):
				t.Fatalf("a %d-byte view answered %d bytes (%v), want it refused as too large", tc.size, len(answer), err)
			case !tc.refused && (err != nil || len(answer) < tc.size):
				t.Fatalf("a %d-byte view answered %d bytes (%v), want it whole", tc.size, len(answer), err)
			}
		})
	}
}

// Only an agent GET route whose verb is read_view is a view. A human-only row
// or a write that names the verb by mistake is never served.
func TestOnlyAnAgentReadIsAView(t *testing.T) {
	views := readViews(map[string]agentPolicy{
		"GET /v1/companies/{id}/360":    {Op: "getCompany360", Access: accessTool, Tool: agents.ReadViewTool},
		"GET /v1/installation":          {Op: "getInstallationSettings", Access: accessHumanOnly, Tool: agents.ReadViewTool},
		"POST /v1/companies/{id}/merge": {Op: "mergeCompany", Access: accessTool, Tool: agents.ReadViewTool},
	})
	if len(views) != 1 || views[0].Name != "company_360" {
		t.Fatalf("the views are %+v, want company_360 alone", views)
	}
	for _, view := range readViews(agentPolicies) {
		route := http.MethodGet + " " + view.Pattern
		if pol := agentPolicies[route]; pol.Access != accessTool || pol.Tool != agents.ReadViewTool {
			t.Errorf("view %s serves %s, whose policy is %+v", view.Name, route, pol)
		}
	}
}

// A path argument is one segment however it is spelled, so it cannot reach a
// second route. A list reaches a query parameter as repeated values.
func TestAViewArgumentStaysInItsPlace(t *testing.T) {
	var gotID string
	var gotQuery url.Values
	router := chi.NewRouter()
	router.Get("/v1/companies/{id}/360", func(w http.ResponseWriter, r *http.Request) {
		gotID, gotQuery = chi.URLParam(r, "id"), r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("answering: %v", err)
		}
	})
	view := agents.ReadView{Name: "company_360", Pattern: "/v1/companies/{id}/360", PathParams: []string{"id"}}
	query := url.Values{"tag": {"a", "b"}}
	if _, err := readViewThrough(&viewDoor{api: router})(context.Background(), view,
		map[string]string{"id": "../../users"}, query); err != nil {
		t.Fatalf("reading the view: %v", err)
	}
	if gotID != url.PathEscape("../../users") || !slices.Equal(gotQuery["tag"], []string{"a", "b"}) {
		t.Errorf("the route read id %q and tag %v, want the argument as one segment and both tags", gotID, gotQuery["tag"])
	}
}

// A route's refusal keeps its kind through the tool.
func TestAViewRefusalKeepsItsKind(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusUnauthorized: apperrors.ErrPermissionDenied, http.StatusForbidden: apperrors.ErrPermissionDenied,
		http.StatusNotFound: apperrors.ErrNotFound, http.StatusConflict: apperrors.ErrConflict,
		http.StatusPreconditionFailed: apperrors.ErrVersionSkew, http.StatusTooManyRequests: apperrors.ErrBudgetExceeded,
	} {
		door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })}
		_, err := readViewThrough(door)(context.Background(), agents.ReadView{Pattern: "/v1/x"}, nil, nil)
		if !errors.Is(err, want) {
			t.Errorf("a route answering %d surfaced as %v, want %v", status, err, want)
		}
	}
	failing := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})}
	_, err := readViewThrough(failing)(context.Background(), agents.ReadView{Pattern: "/v1/x"}, nil, nil)
	var detailed *httperr.DetailedError
	if err == nil || errors.As(err, &detailed) {
		t.Errorf("a route failing with 500 surfaced as %v, want a server fault, not the caller's bad argument", err)
	}
}
