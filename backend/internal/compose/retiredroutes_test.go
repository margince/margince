// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

const retiredResponseRef = "#/components/responses/Retired"

// deprecatedOperation is one `deprecated: true` operation of api/crm.yaml.
type deprecatedOperation struct {
	id, method, route string
	retired           bool
}

// deprecatedOperations reads every deprecated operation of api/crm.yaml. One
// that declares the shared Retired response is retired; every other one is
// still served.
func deprecatedOperations(t *testing.T) []deprecatedOperation {
	t.Helper()
	src, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	var ops []deprecatedOperation
	for path, item := range doc.Paths {
		for _, key := range httpMethods {
			node, present := item[key]
			if !present {
				continue
			}
			//nolint:tagliatelle // OpenAPI's own field name is camelCase.
			var op struct {
				ID         string `yaml:"operationId"`
				Deprecated bool   `yaml:"deprecated"`
				Responses  map[string]struct {
					Ref string `yaml:"$ref"`
				} `yaml:"responses"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", key, path, err)
			}
			retired := op.Responses["410"].Ref == retiredResponseRef
			if retired && !op.Deprecated {
				t.Errorf("%s declares the Retired response and is not deprecated", op.ID)
			}
			if op.Deprecated {
				ops = append(ops, deprecatedOperation{id: op.ID, method: strings.ToUpper(key), route: "/v1" + path, retired: retired})
			}
		}
	}
	return ops
}

// retiredHandlers names every Server method in this package that answers the
// retired refusal, read from the source so a new caller cannot be missed.
func retiredHandlers(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("listing package compose: %v", err)
	}
	handlers := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, isCall := n.(*ast.CallExpr); isCall {
					if ident, named := call.Fun.(*ast.Ident); named && ident.Name == "answerRetired" {
						handlers[fn.Name.Name] = true
					}
				}
				return true
			})
		}
	}
	return handlers
}

// A deprecated operation stays callable, except one the contract declares
// retired. That one answers its declared 410 through the real route table,
// and no other operation's handler gives that answer.
func TestEveryRetiredOperationAnswersItsDeclaredRefusal(t *testing.T) {
	api := crmcontracts.HandlerWithOptions(Server{}, crmcontracts.ChiServerOptions{BaseURL: "/v1"})
	answering := retiredHandlers(t)
	declared := map[string]bool{}
	for _, op := range deprecatedOperations(t) {
		handler := strings.ToUpper(op.id[:1]) + op.id[1:]
		if !op.retired {
			if answering[handler] {
				t.Errorf("%s answers the retired refusal and the contract does not declare it retired", op.id)
			}
			continue
		}
		declared[handler] = true
		route := pathParameter.ReplaceAllString(op.route, "0193c9a0-0000-7000-8000-000000000001")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(op.method, route, strings.NewReader("{}")))
		var problem struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
			t.Errorf("%s %s answered %d with a body that is no problem: %s", op.method, route, rec.Code, rec.Body)
			continue
		}
		if rec.Code != http.StatusGone || problem.Code != "endpoint_retired" {
			t.Errorf("%s %s answered %d %q, want 410 endpoint_retired", op.method, route, rec.Code, problem.Code)
		}
	}
	if len(declared) == 0 {
		t.Fatal("the contract walk found no retired operation; the census is short")
	}
	for handler := range answering {
		if !declared[handler] {
			t.Errorf("%s answers the retired refusal and is no deprecated operation the contract declares retired", handler)
		}
	}
}

func TestARetiredForwardMeasureSavesAsTheMeasureItWasBuiltOn(t *testing.T) {
	for sent, want := range map[string]string{"qualified_likely": "likely", "qualified_weighted": "weighted", "manager_call": "manager_call", "qualified_manager_call": "qualified_manager_call"} {
		measure := crmcontracts.UpdateInstallationSettingsRequestForecastForwardMeasure(sent)
		if got := savedForwardMeasure(&measure); got == nil || *got != want {
			t.Errorf("%s saved as %v, want %s", sent, got, want)
		}
	}
	if savedForwardMeasure[string](nil) != nil {
		t.Error("an absent measure became a saved one")
	}
}
