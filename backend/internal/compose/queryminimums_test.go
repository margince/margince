// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func minimumRouter() http.Handler {
	r := chi.NewRouter()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for route := range queryMinimumTable {
		method, pattern, _ := strings.Cut(route, " ")
		r.With(refuseQueryBelowMinimum).Method(method, pattern, ok)
	}
	return r
}

func TestEveryIntegerQueryParameterRefusesAValueBelowItsMinimum(t *testing.T) {
	h := minimumRouter()
	for route, minimums := range queryMinimumTable {
		for _, m := range minimums {
			below := strconv.FormatInt(m.Minimum-1, 10)
			rec := call(h, route, url.Values{m.Name: {below}}.Encode())
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s ?%s=%s answered %d, want 422", route, m.Name, below, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), m.Name) {
				t.Errorf("%s ?%s refusal does not name the parameter: %s", route, m.Name, rec.Body.String())
			}
			at := strconv.FormatInt(m.Minimum, 10)
			if rec := call(h, route, url.Values{m.Name: {at}}.Encode()); rec.Code != http.StatusOK {
				t.Errorf("%s ?%s=%s answered %d, want 200: the minimum itself is allowed", route, m.Name, at, rec.Code)
			}
		}
	}
}

// The page size is the parameter the clamp hid, on every list route.
func TestAPageSizeBelowOneIsRefusedOnTheListRoutes(t *testing.T) {
	h := minimumRouter()
	for _, route := range []string{"GET /v1/contacts", "GET /v1/companies", "GET /v1/deals", "GET /v1/activities", "GET /v1/leads", "GET /v1/projects"} {
		for _, limit := range []string{"0", "-1"} {
			if rec := call(h, route, "limit="+limit); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s?limit=%s answered %d, want 422", route, limit, rec.Code)
			}
		}
	}
}

// A value that is no integer belongs to the binding, which refuses it with the
// parameter's type. This middleware must not answer first with a minimum.
func TestANonIntegerIsLeftToTheBinding(t *testing.T) {
	if rec := call(minimumRouter(), "GET /v1/contacts", "limit=abc"); rec.Code != http.StatusOK {
		t.Errorf("?limit=abc answered %d, want it passed on to the binding", rec.Code)
	}
}
