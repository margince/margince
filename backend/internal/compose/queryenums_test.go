// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// enumRouter mounts every route the table names behind the middleware. As in
// the generated router the match comes first, so the middleware can read the
// pattern.
func enumRouter() http.Handler {
	r := chi.NewRouter()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for route := range queryEnumTable {
		method, pattern, _ := strings.Cut(route, " ")
		r.With(refuseUnknownQueryEnum).Method(method, pattern, ok)
	}
	return r
}

func call(h http.Handler, route, rawQuery string) *httptest.ResponseRecorder {
	method, pattern, _ := strings.Cut(route, " ")
	// A path template stands for any id; a literal is enough to match it.
	path := strings.NewReplacer("{", "", "}", "").Replace(pattern)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path+"?"+rawQuery, nil))
	return rec
}

func TestEveryEnumQueryParameterRefusesAValueOutsideItsEnum(t *testing.T) {
	h := enumRouter()
	for route, enums := range queryEnumTable {
		for _, e := range enums {
			rec := call(h, route, url.Values{e.Name: {"zz_not_a_value"}}.Encode())
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s ?%s=zz_not_a_value answered %d, want 422", route, e.Name, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), e.Name) {
				t.Errorf("%s ?%s refusal does not name the parameter: %s", route, e.Name, rec.Body.String())
			}
		}
	}
}

func TestEveryEnumQueryParameterAdmitsEachOfItsValues(t *testing.T) {
	h := enumRouter()
	for route, enums := range queryEnumTable {
		for _, e := range enums {
			for _, v := range e.Values {
				if rec := call(h, route, url.Values{e.Name: {v}}.Encode()); rec.Code != http.StatusOK {
					t.Errorf("%s ?%s=%s answered %d, want 200", route, e.Name, v, rec.Code)
				}
			}
		}
	}
}

func TestACommaListOnASingleValueParameterIsRefused(t *testing.T) {
	h := enumRouter()
	for _, tc := range []struct{ route, query string }{
		{"GET /v1/worklist", "scope=mine,team"},
		{"GET /v1/approvals", "status=pending,approved"},
		{"GET /v1/deals", "status=open,won"},
	} {
		if rec := call(h, tc.route, tc.query); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s?%s answered %d, want 422: a comma is part of the value there", tc.route, tc.query, rec.Code)
		}
	}
}

func TestAnExplodedArrayTakesRepeatedKeysAndNotAJoinedValue(t *testing.T) {
	h := enumRouter()
	const route = "GET /v1/me/ai-activity"
	if rec := call(h, route, "kinds=morning_brief&kinds=document_extract"); rec.Code != http.StatusOK {
		t.Errorf("repeated keys answered %d, want 200", rec.Code)
	}
	if rec := call(h, route, "kinds=morning_brief&kinds=zz"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a repeated key with an unknown member answered %d, want 422", rec.Code)
	}
	// An exploded array binds "a,b" as one element, which no member equals.
	if rec := call(h, route, "kinds=morning_brief,document_extract"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a joined value on an exploded array answered %d, want 422", rec.Code)
	}
}

func TestAnArrayThatIsNotExplodedTakesAJoinedValue(t *testing.T) {
	var found bool
	h := enumRouter()
	for route, enums := range queryEnumTable {
		for _, e := range enums {
			if !e.Array || e.Explode {
				continue
			}
			found = true
			joined := e.Values[0] + "," + e.Values[len(e.Values)-1]
			if rec := call(h, route, url.Values{e.Name: {joined}}.Encode()); rec.Code != http.StatusOK {
				t.Errorf("%s ?%s=%s answered %d, want 200", route, e.Name, joined, rec.Code)
			}
			if rec := call(h, route, url.Values{e.Name: {e.Values[0] + ",zz"}}.Encode()); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s ?%s with an unknown joined member answered %d, want 422", route, e.Name, rec.Code)
			}
		}
	}
	if !found {
		t.Skip("the contract declares no array enum parameter that is not exploded")
	}
}

func TestAnEmptyEnumQueryValueIsRefusedNotReadAsAbsent(t *testing.T) {
	rec := call(enumRouter(), "GET /v1/forecast/movement", "reading=")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an empty reading answered %d, want 422", rec.Code)
	}
}

func TestDeletesTakeTheirEnumQueryParametersToo(t *testing.T) {
	h := enumRouter()
	for _, tc := range []struct{ route, param string }{
		{"DELETE /v1/ai-model-rates", "lane"},
		{"DELETE /v1/activities/{id}/disposition", "scope"},
	} {
		if rec := call(h, tc.route, tc.param+"=zz_not_a_value"); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s ?%s answered %d, want 422", tc.route, tc.param, rec.Code)
		}
	}
}

// The expected set comes from a second, separate reading of the contract.
// A table the generator wrote short cannot vouch for itself.
func TestTheTableHoldsEveryEnumQueryParameterTheContractDeclares(t *testing.T) {
	want := contractEnumQueryParameters(t)
	got := map[string]bool{}
	for route, enums := range queryEnumTable {
		for _, e := range enums {
			got[route+" ?"+e.Name] = true
		}
	}
	if len(want) < 50 {
		t.Fatalf("the contract reading found only %d enum query parameters; the reader is short", len(want))
	}
	for key := range want {
		if !got[key] {
			t.Errorf("the contract declares %s as an enum and the generated table lacks it", key)
		}
	}
	for key := range got {
		if !want[key] {
			t.Errorf("the generated table holds %s and the contract declares no such enum", key)
		}
	}
}

// contractEnumQueryParameters walks crm.yaml as plain YAML and resolves local
// $refs by hand. It returns "METHOD /v1/path ?name" for each query parameter
// whose schema, or array items, is an enum of strings.
func contractEnumQueryParameters(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	resolve := func(node any) map[string]any {
		for range 8 {
			m, ok := node.(map[string]any)
			if !ok {
				return nil
			}
			ref, isRef := m["$ref"].(string)
			if !isRef {
				return m
			}
			node = doc
			for part := range strings.SplitSeq(strings.TrimPrefix(ref, "#/"), "/") {
				next, ok := node.(map[string]any)
				if !ok {
					return nil
				}
				node = next[part]
			}
		}
		return nil
	}
	isStringEnum := func(schema map[string]any) bool {
		if schema == nil {
			return false
		}
		if items := resolve(schema["items"]); items != nil {
			schema = items
		}
		values, ok := schema["enum"].([]any)
		if !ok || len(values) == 0 {
			return false
		}
		for _, v := range values {
			if _, ok := v.(string); !ok {
				return false
			}
		}
		return true
	}
	found := map[string]bool{}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		entry, _ := item.(map[string]any)
		inherited, _ := entry["parameters"].([]any)
		for method, op := range entry {
			operation, ok := op.(map[string]any)
			if !ok || !isHTTPMethod(method) {
				continue
			}
			own, _ := operation["parameters"].([]any)
			for _, p := range append(append([]any{}, inherited...), own...) {
				param := resolve(p)
				if param == nil || param["in"] != "query" {
					continue
				}
				name, _ := param["name"].(string)
				if isStringEnum(resolve(param["schema"])) {
					found[strings.ToUpper(method)+" /v1"+path+" ?"+name] = true
				}
			}
		}
	}
	return found
}

func isHTTPMethod(s string) bool {
	switch s {
	case "get", "put", "post", "delete", "patch", "head", "options":
		return true
	default:
		return false
	}
}
