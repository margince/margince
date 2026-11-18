// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package queryguard

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

func router() http.Handler {
	r := chi.NewRouter()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	guard := Refuse(func(r *http.Request) string { return chi.RouteContext(r.Context()).RoutePattern() })
	for route := range Declared {
		method, pattern, _ := strings.Cut(route, " ")
		r.With(guard).Method(method, pattern, ok)
	}
	return r
}

func call(h http.Handler, route, rawQuery string) *httptest.ResponseRecorder {
	method, pattern, _ := strings.Cut(route, " ")
	path := strings.NewReplacer("{", "", "}", "").Replace(pattern)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path+"?"+rawQuery, nil))
	return rec
}

func TestEveryRouteRefusesAKeyItDoesNotDeclareAndAdmitsEachDeclaredOne(t *testing.T) {
	h := router()
	for route, names := range Declared {
		rec := call(h, route, "zz_not_declared=1")
		if rec.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(rec.Body.String(), "zz_not_declared") || !strings.Contains(rec.Body.String(), "invalid_parameter") {
			t.Errorf("%s ?zz_not_declared answered %d %s, want 422 naming the key", route, rec.Code, rec.Body.String())
		}
		for _, name := range names {
			if rec := call(h, route, url.Values{name: {"1"}}.Encode()); rec.Code != http.StatusOK {
				t.Errorf("%s ?%s answered %d, want 200", route, name, rec.Code)
			}
		}
	}
}

func TestEveryUndeclaredKeyIsNamedInOneRefusal(t *testing.T) {
	rec := call(router(), "GET /v1/deals", "limit=5&stauts=open&zzz=1")
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "stauts") || !strings.Contains(body, "zzz") || strings.Contains(body, `"limit"`) {
		t.Fatalf("answered %d %s, want 422 naming stauts and zzz only", rec.Code, body)
	}
}

func TestCustomFieldFiltersAndUnlistedRoutesAreLeftAlone(t *testing.T) {
	if rec := call(router(), "GET /v1/deals", "cf_score=3"); rec.Code != http.StatusOK {
		t.Errorf("a cf_ filter answered %d, want 200", rec.Code)
	}
	if err := Undeclared("GET /oauth/authorize", "client_id=x&anything=1"); err != nil {
		t.Errorf("a route with no contract entry was refused: %v", err)
	}
}

func TestAMalformedQueryStringIsRefusedNotReadAsNoFilter(t *testing.T) {
	h := router()
	for _, query := range []string{"st%ZZatus=open", "status=%ZZ", "limit=5&%=1", "a=1;b=2"} {
		rec := call(h, "GET /v1/deals", query)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "invalid_parameter") {
			t.Errorf("?%s answered %d %s, want 422 invalid_parameter", query, rec.Code, rec.Body.String())
		}
	}
}

// The expected set comes from a plain reading of the contract, so a table the
// generator wrote short cannot vouch for itself.
func TestTheTableHoldsEveryQueryParameterOfEveryCheckedOperation(t *testing.T) {
	raw, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Content[0]
	components := lookup(lookup(root, "components"), "parameters")
	want := map[string]bool{}
	paths := lookup(root, "paths")
	for i := 0; i+1 < len(paths.Content); i += 2 {
		item := deref(paths.Content[i+1])
		for j := 0; j+1 < len(item.Content); j += 2 {
			method := strings.ToUpper(item.Content[j].Value)
			op := deref(item.Content[j+1])
			if !isMethod(method) || skipped(op) {
				continue
			}
			for _, list := range []*yaml.Node{lookup(item, "parameters"), lookup(op, "parameters")} {
				if list == nil {
					continue
				}
				for _, p := range deref(list).Content {
					p = resolve(p, components)
					if lookup(p, "in") != nil && lookup(p, "in").Value == "query" {
						want[method+" /v1"+paths.Content[i].Value+" ?"+lookup(p, "name").Value] = true
					}
				}
			}
		}
	}
	got := map[string]bool{}
	for route, names := range Declared {
		for _, name := range names {
			got[route+" ?"+name] = true
		}
	}
	if len(want) < 400 {
		t.Fatalf("the contract reading found only %d checked query parameters; the reader is short", len(want))
	}
	for key := range want {
		if !got[key] {
			t.Errorf("the contract declares %s and the generated table lacks it", key)
		}
	}
	for key := range got {
		if !want[key] {
			t.Errorf("the generated table holds %s and the contract declares no such parameter", key)
		}
	}
}

func lookup(node *yaml.Node, key string) *yaml.Node {
	node = deref(node)
	if node == nil {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// deref follows a YAML alias, which the contract uses to share a parameter list.
func deref(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

func resolve(p, components *yaml.Node) *yaml.Node {
	if ref := lookup(p, "$ref"); ref != nil {
		return lookup(components, ref.Value[strings.LastIndex(ref.Value, "/")+1:])
	}
	return p
}

func isMethod(m string) bool {
	switch m {
	case "GET", "PUT", "POST", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	}
	return false
}

// skipped is an operation the server leaves unchecked: anonymous, or marked open.
func skipped(op *yaml.Node) bool {
	if open := lookup(op, "x-open-query"); open != nil && open.Value == "true" {
		return true
	}
	security := deref(lookup(op, "security"))
	return security != nil && security.Kind == yaml.SequenceNode && len(security.Content) == 0
}
