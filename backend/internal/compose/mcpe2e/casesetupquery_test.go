// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const e2eLLMCaseSetup = "../../../../e2e/llm/casesetup.py"

var (
	// A GET the setup steps make with a query string: the path, then the query.
	setupGetWithQuery = regexp.MustCompile(`call\("GET", f?"([^"?]*)\?([^"]*)"`)
	// The list paths handed to the one helper whose path is a variable.
	setupNamedListPath = regexp.MustCompile(`_one_named\(admin, "([^"]+)"`)
)

// The API refuses a query parameter its operation does not declare. A setup step
// that sends one stops every run of its case before the model is asked.
func TestTheLaneSetupStepsSendOnlyQueryParametersTheContractTakes(t *testing.T) {
	source, err := os.ReadFile(e2eLLMCaseSetup)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	named := setupNamedListPath.FindAllStringSubmatch(string(source), -1)
	checked := 0
	for _, call := range setupGetWithQuery.FindAllStringSubmatch(string(source), -1) {
		paths := []string{call[1]}
		if call[1] == "{path}" {
			paths = paths[:0]
			for _, n := range named {
				paths = append(paths, n[1])
			}
		}
		for _, path := range paths {
			for _, param := range queryNames(call[2]) {
				checked++
				if !getTakesQuery(doc, path, param) {
					t.Errorf("%s sends GET %s?%s=…, and the contract declares no such query parameter there",
						e2eLLMCaseSetup, path, param)
				}
			}
		}
	}
	// The census must see the calls it exists for, or a changed spelling passes it.
	if checked < 6 {
		t.Fatalf("read %d query parameters from %s; the regexes no longer match its calls", checked, e2eLLMCaseSetup)
	}
}

// queryNames reads the parameter names of a query string whose values may be
// f-string placeholders.
func queryNames(query string) []string {
	var out []string
	for pair := range strings.SplitSeq(query, "&") {
		name, _, _ := strings.Cut(pair, "=")
		if unescaped, err := url.QueryUnescape(name); err == nil && unescaped != "" {
			out = append(out, unescaped)
		}
	}
	return out
}

func getTakesQuery(doc *openapi3.T, path, param string) bool {
	item := doc.Paths.Find(path)
	if item == nil || item.Get == nil {
		return false
	}
	for _, params := range []openapi3.Parameters{item.Parameters, item.Get.Parameters} {
		for _, ref := range params {
			if ref.Value != nil && ref.Value.In == openapi3.ParameterInQuery && ref.Value.Name == param {
				return true
			}
		}
	}
	return false
}
