// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A field the contract declares as an array is never written null. A Go nil
// slice is written null, so a record with nothing in it is where this breaks.
//
// The corpus is every read in crm.yaml that a fresh workspace can answer. A new
// route is covered without anyone listing it.
func TestNoReadWritesNullWhereTheContractDeclaresAnArray(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Array Shapes", "admin@arrayshapes.test", "Admin")
	owners := seedOneOfEachParent(t, e)

	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading api/crm.yaml: %v", err)
	}

	paths := doc.Paths.Map()
	routes := make([]string, 0, len(paths))
	for route := range paths {
		routes = append(routes, route)
	}
	sort.Strings(routes)

	read, skipped := 0, 0
	for _, route := range routes {
		op := paths[route].Get
		if op == nil || hasRequiredQuery(op) {
			continue
		}
		url, ok := resolveParents(route, owners)
		if !ok {
			skipped++
			continue
		}
		body, ok := getJSON(t, e, "/v1"+url)
		if !ok {
			skipped++
			continue
		}
		schema := okSchema(op)
		if schema == nil {
			continue
		}
		read++
		for _, at := range nullArrays(schema, body, "$") {
			t.Errorf("GET %s answers null at %s, where the contract declares an array", url, at)
		}
	}
	t.Logf("walked %d reads, skipped %d", read, skipped)
	// A short census reads as a pass: the walk must have reached a real share of the reads.
	if read < 60 {
		t.Fatalf("only %d reads were walked (%d skipped for a missing parent or a refusal): the corpus is too small to prove anything", read, skipped)
	}
}

// seedOneOfEachParent creates the parents most routes hang off and returns
// their ids by the collection segment that names them.
func seedOneOfEachParent(t *testing.T, e *apptest.AppEnv) map[string]string {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	owners := map[string]string{"pipelines": stages.PipelineID}
	owners["deals"] = apptest.CreateOpenDeal(t, e, stages)
	owners["companies"] = createdID(t, e, "/v1/companies", AnyMap{"display_name": "Bare Works", "source": "manual"})
	owners["contacts"] = createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Bare Person", "source": "manual"})
	owners["leads"] = createdID(t, e, "/v1/leads", AnyMap{"full_name": "Bare Lead", "email": "bare@lead.test", "source": "manual"})
	owners["projects"] = createdID(t, e, "/v1/projects", AnyMap{"name": "Bare Project", "company_id": owners["companies"], "source": "manual"})
	owners["activities"] = createdID(t, e, "/v1/activities", AnyMap{"kind": "task", "subject": "Bare task"})
	return owners
}

// getJSON reads one route and returns its body when the answer is a 200 with a
// JSON document. A file or a refusal is not a shape this test judges.
//
//craft:ignore naked-any the body is whatever JSON document the route answers
func getJSON(t *testing.T, e *apptest.AppEnv, path string) (any, bool) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, e.TS.URL+path, nil)
	if err != nil {
		t.Fatalf("building GET %s: %v", path, err)
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer apptest.CloseBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		return nil, false
	}
	var body any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GET %s: decoding the body: %v", path, err)
	}
	return body, body != nil
}

func hasRequiredQuery(op *openapi3.Operation) bool {
	for _, param := range op.Parameters {
		if param.Value != nil && param.Value.In == "query" && param.Value.Required {
			return true
		}
	}
	return false
}

// resolveParents fills a route's one path parameter with the seeded record of
// the collection named just before it. A route with two parameters, or one that
// follows a collection nobody seeded, is not reachable here.
func resolveParents(route string, owners map[string]string) (string, bool) {
	segments := strings.Split(route, "/")
	for i, segment := range segments {
		if !strings.HasPrefix(segment, "{") {
			continue
		}
		if i == 0 || strings.Count(route, "{") != 1 {
			return "", false
		}
		id, ok := owners[segments[i-1]]
		if !ok {
			return "", false
		}
		segments[i] = id
	}
	return strings.Join(segments, "/"), true
}

func okSchema(op *openapi3.Operation) *openapi3.Schema {
	ok := op.Responses.Status(http.StatusOK)
	if ok == nil || ok.Value == nil {
		return nil
	}
	media := ok.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil {
		return nil
	}
	return media.Schema.Value
}

// nullArrays walks a response beside its schema and names every place a JSON
// null meets a schema whose type is array and not nullable. Alternatives (oneOf,
// anyOf) are not walked, since the value does not say which branch it took.
//
//craft:ignore naked-any the value is a decoded JSON document of any shape
func nullArrays(schema *openapi3.Schema, value any, at string) []string {
	if schema == nil {
		return nil
	}
	if value == nil {
		if schema.Type != nil && schema.Type.Is("array") {
			return []string{at}
		}
		return nil
	}
	var found []string
	for _, part := range schema.AllOf {
		found = append(found, nullArrays(part.Value, value, at)...)
	}
	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			if property, declared := schema.Properties[name]; declared {
				found = append(found, nullArrays(property.Value, child, at+"."+name)...)
			}
		}
	case []any:
		if schema.Items != nil {
			for _, child := range typed {
				found = append(found, nullArrays(schema.Items.Value, child, at+"[]")...)
			}
		}
	}
	return found
}

// The walk names a null under an array declaration, in a nested object and
// through allOf, and leaves a nullable array alone.
func TestTheArrayWalkNamesANullOnlyWhereAnArrayIsDeclared(t *testing.T) {
	array := openapi3.NewArraySchema()
	nullable := &openapi3.Schema{Type: &openapi3.Types{"array", "null"}}
	inner := openapi3.NewObjectSchema().WithProperty("tags", array)
	outer := openapi3.NewObjectSchema().
		WithProperty("owner", inner).
		WithProperty("maybe", nullable)
	merged := &openapi3.Schema{AllOf: openapi3.SchemaRefs{{Value: outer}}}

	value := map[string]any{"owner": map[string]any{"tags": nil}, "maybe": nil}
	got := nullArrays(merged, value, "$")
	if len(got) != 1 || got[0] != "$.owner.tags" {
		t.Errorf("nullArrays = %v, want only $.owner.tags", got)
	}
	if got := nullArrays(merged, map[string]any{"owner": map[string]any{"tags": []any{}}}, "$"); len(got) != 0 {
		t.Errorf("an empty array was named: %v", got)
	}
}
