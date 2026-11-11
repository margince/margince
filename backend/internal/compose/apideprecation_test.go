// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
	"github.com/margince/margince/backend/internal/platform/apideprecation"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

func TestEveryDeprecatedOperationAnnouncesThePolicyThroughTheGeneratedRouter(t *testing.T) {
	stop := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	}
	router := contractRouter(Server{}, stop)
	doc := loadContract(t)
	expected := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if !op.Deprecated {
				continue
			}
			route := method + " " + httpserver.BaseURL + path
			expected[route] = true
			if _, ok := deprecationTable[route]; !ok {
				t.Errorf("missing notice for %s", route)
			}
			requestPath := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(httpserver.BaseURL+path, "00000000-0000-0000-0000-000000000001")
			query, header := url.Values{}, http.Header{}
			// A required parameter's refusal comes before the operation. Every required
			// query parameter on a deprecated operation takes a uuid.
			for _, param := range op.Parameters {
				if p := param.Value; p != nil && p.Required && p.In == "query" {
					query.Set(p.Name, "00000000-0000-0000-0000-000000000001")
				} else if p != nil && p.Required && p.In == "header" {
					header.Set(p.Name, `"1"`)
				}
			}
			if len(query) > 0 {
				requestPath += "?" + query.Encode()
			}
			request := httptest.NewRequest(method, requestPath, nil)
			request.Header = header
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Errorf("%s did not reach operation middleware: %d", route, response.Code)
			}
			if response.Header().Get("Link") != apideprecation.PolicyLink {
				t.Errorf("%s status=%d missing policy link", route, response.Code)
			}
		}
	}
	if len(expected) == 0 || len(expected) != len(deprecationTable) {
		t.Fatalf("contract notices=%d, generated=%d", len(expected), len(deprecationTable))
	}
	response := call(router, "GET /v1/contacts", "")
	if response.Header().Get("Link") != "" {
		t.Fatal("ordinary route marked deprecated")
	}
}

func TestAParameterRefusalAlsoAnnouncesDeprecation(t *testing.T) {
	router := contractRouter(Server{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/companies/not-a-uuid/brief", nil))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Header().Get("Link"), "deprecation") {
		t.Fatalf("refusal status=%d headers=%v", response.Code, response.Header())
	}
}

func TestThePublishedDeprecationPolicyMatchesTheSharedRules(t *testing.T) {
	doc := loadContract(t)
	raw, err := json.Marshal(doc.Info.Extensions[apideprecation.PolicyExtension])
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		WindowMonths  int    `json:"window_months"`
		PatchRemovals bool   `json:"patch_removals"`
		Announcement  string `json:"announcement"`
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	if policy.WindowMonths != apideprecation.WindowMonths || policy.PatchRemovals || policy.Announcement != "release-publication" {
		t.Fatalf("contract policy=%+v", policy)
	}
	page, err := os.ReadFile("../../../docs/reference/api-compatibility.md")
	if err != nil {
		t.Fatal(err)
	}
	phrase := fmt.Sprintf("at least %d calendar months", apideprecation.WindowMonths)
	if !strings.Contains(doc.Info.Description, phrase) || !strings.Contains(string(page), phrase) {
		t.Fatal("published notice window differs from the shared rule")
	}
}

func TestTheLinkedContractListsEveryDeprecatedOperation(t *testing.T) {
	builder := agentbundle.Builder{}
	body, err := builder.SpecJSON("https://api.example.test/v1")
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Info map[string]json.RawMessage `json:"info"`
	}
	if err := json.Unmarshal(body, &published); err != nil {
		t.Fatal(err)
	}
	var notices map[string]apideprecation.Notice
	if err := json.Unmarshal(published.Info[apideprecation.NoticesExtension], &notices); err != nil {
		t.Fatal(err)
	}
	if len(notices) != len(deprecationTable) {
		t.Fatalf("linked notices=%d, served notices=%d", len(notices), len(deprecationTable))
	}
	for route := range deprecationTable {
		if _, ok := notices[route]; !ok {
			t.Errorf("linked document omits %s", route)
		}
	}
}
