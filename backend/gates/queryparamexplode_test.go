// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every query parameter in crm.yaml is encoded the way the frontend client sends it.
//
// openapi-fetch, left at its default serializer, repeats an array's name once
// per value (`team_ids=a&team_ids=b`). A parameter declared `explode: false`
// makes the generated binder demand one comma-joined value and refuse the
// repeat with a 422. The screen then works for one value and breaks for two.

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// commaJoinedQueryParams keep `explode: false`, keyed "<path or component> <parameter>".
var commaJoinedQueryParams = gatekit.Waive(map[string]string{
	"/search types": "a published spelling that API callers send as `types=contact,company` " +
		"(searchdoors_integration_test.go); the frontend sends one type per request",
})

func TestQueryParametersAreEncodedTheWayTheClientSendsThem(t *testing.T) {
	t.Parallel()
	client, err := os.ReadFile("../frontend/src/api/client.ts")
	if err != nil {
		t.Fatalf("read the frontend client: %v", err)
	}
	if strings.Contains(string(client), "querySerializer") {
		t.Fatal("frontend/src/api/client.ts now sets a querySerializer; this gate assumes " +
			"openapi-fetch's default exploded arrays, so derive the expected encoding from " +
			"the serializer instead")
	}

	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("read crm.yaml: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse crm.yaml: %v", err)
	}
	defer commaJoinedQueryParams.AssertAllMatched(t)
	var queryParams int
	root := doc.Content[0]
	scopes := append(mappingPairs(mappingValue(root, "paths")),
		mappingPairs(mappingValue(mappingValue(root, "components"), "parameters"))...)
	for _, scope := range scopes {
		walkQueryParameters(scope[1], func(param *yaml.Node) {
			queryParams++
			explode := mappingValue(param, "explode")
			if explode == nil || explode.Value != "false" {
				return
			}
			subject := scope[0].Value + " " + mappingValue(param, "name").Value
			if !commaJoinedQueryParams.Waived(t, subject) {
				t.Errorf("crm.yaml:%d: query parameter %q declares explode: false; the frontend "+
					"client sends repeated values, so remove it", param.Line, subject)
			}
		})
	}
	// A walk that found few parameters has stopped reading the contract.
	if queryParams < 200 {
		t.Fatalf("found %d query parameters in crm.yaml, far fewer than it declares", queryParams)
	}
}

// walkQueryParameters visits every mapping under node that declares `in: query`.
func walkQueryParameters(node *yaml.Node, visit func(*yaml.Node)) {
	if in := mappingValue(node, "in"); in != nil && in.Value == "query" {
		visit(node)
	}
	for _, child := range node.Content {
		walkQueryParameters(child, visit)
	}
}
