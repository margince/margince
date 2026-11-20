// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

var routeParam = regexp.MustCompile(`\{[^}]+\}`)

// Every GET the contract declares is a route the /v1 router serves, and every
// one of them answers with a Cache-Control even when its handler sets none. The
// route set comes from the router itself, held against the contract so a walk
// that finds fewer routes than exist fails instead of passing.
func TestEveryContractReadAnswersWithACacheControl(t *testing.T) {
	router := contractRouter(crmcontracts.Unimplemented{})
	var walked []string
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet {
			walked = append(walked, route)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the /v1 router: %v", err)
	}
	var declared []string
	for path, item := range loadContract(t).Paths.Map() {
		if item.Get != nil {
			declared = append(declared, httpserver.BaseURL+path)
		}
	}
	slices.Sort(walked)
	slices.Sort(declared)
	if !slices.Equal(walked, declared) {
		t.Fatalf("the router serves %d GET routes and the contract declares %d; the walk must see every read",
			len(walked), len(declared))
	}
	for _, route := range walked {
		path := routeParam.ReplaceAllString(route, "00000000-0000-0000-0000-000000000001")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Cache-Control"); got != apiDefaultCache {
			t.Errorf("GET %s answered with Cache-Control %q, want %q", route, got, apiDefaultCache)
		}
	}
}

func TestAHandlersOwnCacheControlWinsOverTheDefault(t *testing.T) {
	server := responseCodecServer{serve: func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "private, no-cache")
		w.WriteHeader(http.StatusOK)
	}}
	rec := httptest.NewRecorder()
	contractRouter(server).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
	if got := rec.Header().Get("Cache-Control"); got != "private, no-cache" {
		t.Fatalf("Cache-Control = %q, want the handler's own private, no-cache", got)
	}
}
