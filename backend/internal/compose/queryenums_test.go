// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// enumRouter mounts every route the table names behind the middleware. As in
// the generated router the match comes first, so the middleware can read the
// pattern.
func enumRouter() http.Handler {
	r := chi.NewRouter()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for route := range queryEnumTable {
		_, pattern, _ := strings.Cut(route, " ")
		r.With(refuseUnknownQueryEnum).Method(http.MethodGet, pattern, ok)
	}
	return r
}

func getWith(t *testing.T, h http.Handler, route, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	_, pattern, _ := strings.Cut(route, " ")
	// A path template stands for any id; a literal is enough to match it.
	path := strings.NewReplacer("{", "", "}", "").Replace(pattern)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?"+rawQuery, nil))
	return rec
}

func TestEveryEnumQueryParameterRefusesAValueOutsideItsEnum(t *testing.T) {
	h := enumRouter()
	checked := 0
	for route, enums := range queryEnumTable {
		for _, e := range enums {
			rec := getWith(t, h, route, url.Values{e.Name: {"zz_not_a_value"}}.Encode())
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s ?%s=zz_not_a_value answered %d, want 422", route, e.Name, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), e.Name) {
				t.Errorf("%s ?%s refusal does not name the parameter: %s", route, e.Name, rec.Body.String())
			}
			checked++
		}
	}
	if checked < 50 {
		t.Fatalf("only %d enum query parameters were checked; the table reads short", checked)
	}
}

func TestEveryEnumQueryParameterAdmitsEachOfItsValues(t *testing.T) {
	h := enumRouter()
	for route, enums := range queryEnumTable {
		for _, e := range enums {
			for _, v := range e.Values {
				if rec := getWith(t, h, route, url.Values{e.Name: {v}}.Encode()); rec.Code != http.StatusOK {
					t.Errorf("%s ?%s=%s answered %d, want 200", route, e.Name, v, rec.Code)
				}
			}
		}
	}
}

func TestAnEmptyEnumQueryValueIsRefusedNotReadAsAbsent(t *testing.T) {
	rec := getWith(t, enumRouter(), "GET /v1/forecast/movement", "reading=")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an empty reading answered %d, want 422", rec.Code)
	}
}

func TestAnArrayEnumQueryValueMayRepeatOrJoin(t *testing.T) {
	h := enumRouter()
	for _, q := range []string{"types=contact&types=deal", "types=contact,deal"} {
		if rec := getWith(t, h, "GET /v1/search", q); rec.Code != http.StatusOK {
			t.Errorf("?%s answered %d, want 200", q, rec.Code)
		}
	}
	if rec := getWith(t, h, "GET /v1/search", "types=contact,zz"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a joined list with an unknown member answered %d, want 422", rec.Code)
	}
}

func TestTheIssueRoutesAreInTheEnumTable(t *testing.T) {
	for _, route := range []string{
		"GET /v1/leads", "GET /v1/dedupe/candidates", "GET /v1/activities",
		"GET /v1/relationships", "GET /v1/approvals", "GET /v1/forecast/movement",
	} {
		if len(queryEnumTable[route]) == 0 {
			t.Errorf("%s has no enum query parameter in the generated table", route)
		}
	}
}
