// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A request body whose schema, or one of its nested objects, says
// `minProperties` promises an empty object is refused. An all-optional struct
// decodes `{}` like any real edit, so the handler asks httperr.RequireAnyField,
// or sits in emptyBodyAccepted with the layer that refuses it.

import (
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	storeRefusesEmpty  = "the store refuses a body with neither field, in words that name both"
	configRefusesEmpty = "UpdateConfig refuses an empty configuration; the nested value objects are its settings"
	selectionRefused   = "the nested bound is a category selection: ValidateSelection refuses an empty one"
)

// emptyBodyAccepted are the operations whose handler does not refuse `{}` itself.
var emptyBodyAccepted = gatekit.Waive(map[string]string{
	"UpdateVoiceProfile":       storeRefusesEmpty,
	"UpdateVoiceCorpusSource":  storeRefusesEmpty,
	"UpdateProviderConnection": configRefusesEmpty,
	"ConnectProvider":          selectionRefused,
})

func TestEveryBodyThatBoundsItsPropertiesRefusesAnEmptyObject(t *testing.T) {
	t.Parallel()
	defer emptyBodyAccepted.AssertAllMatched(t)
	doc := loadContractDocument(t)
	schemas := descendContract(t, doc, "components", "schemas")
	ops := minPropertiesOperations(t, doc, schemas)
	for _, handler := range []string{
		"UpdateRole", "UpdateWebhookSubscription", "UpdateVoiceProfile", "UpdateVoiceCorpusSource", "UpdateProviderConnection",
	} {
		found := false
		for _, op := range ops {
			found = found || op.handler == handler
		}
		if !found {
			t.Fatalf("the census found %d operation(s) and not %s; the walk lost its subjects", len(ops), handler)
		}
	}
	handlers := handlerTypeCalls(t)
	var missing []string
	for _, op := range ops {
		calls, found := handlers[op.handler]
		if !found {
			t.Errorf("%s %s: no handler method %s was found, so the census cannot judge it", op.method, op.path, op.handler)
			continue
		}
		if calls["RequireAnyField"] || emptyBodyAccepted.Waived(t, op.handler) {
			continue
		}
		missing = append(missing, op.method+" "+op.path+" "+op.handler)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d operation(s) bound their body's properties and never refuse an empty object:\n\t%s\n\n"+
			"Call httperr.RequireAnyField after the decode, or record which layer refuses it.",
			len(missing), strings.Join(missing, "\n\t"))
	}
}

type minPropertiesOperation struct{ method, path, handler string }

// minPropertiesOperations lists each operation whose JSON body schema carries
// `minProperties`, at the top or in a nested object, with the handler method
// the generated router calls.
func minPropertiesOperations(t *testing.T, doc, schemas map[string]any) []minPropertiesOperation {
	t.Helper()
	var out []minPropertiesOperation
	for path, item := range descendContract(t, doc, "paths") {
		pathItem, _ := item.(map[string]any)
		for _, method := range []string{"post", "put", "patch", "delete"} {
			op, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			schema := bodySchema(op)
			if schema == nil || !boundsProperties(schemas, schema, map[string]bool{}) {
				continue
			}
			id, _ := op["operationId"].(string)
			if id == "" {
				t.Errorf("%s %s has no operationId", strings.ToUpper(method), path)
				continue
			}
			out = append(out, minPropertiesOperation{
				method: strings.ToUpper(method), path: path, handler: strings.ToUpper(id[:1]) + id[1:],
			})
		}
	}
	return out
}

// boundsProperties is whether the schema, a property under it or an allOf part
// carries `minProperties`. seen guards a schema that refers back to itself.
func boundsProperties(schemas, node map[string]any, seen map[string]bool) bool {
	if ref, ok := node["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		target, found := schemas[name].(map[string]any)
		if !found || seen[name] {
			return false
		}
		seen[name] = true
		return boundsProperties(schemas, target, seen)
	}
	if node["minProperties"] != nil {
		return true
	}
	props, _ := node["properties"].(map[string]any)
	for _, prop := range props {
		if p, ok := prop.(map[string]any); ok && boundsProperties(schemas, p, seen) {
			return true
		}
	}
	parts, _ := node["allOf"].([]any)
	for _, part := range parts {
		if p, ok := part.(map[string]any); ok && boundsProperties(schemas, p, seen) {
			return true
		}
	}
	return false
}
