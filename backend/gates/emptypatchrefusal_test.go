// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A request body whose schema says `minProperties` promises an empty object is
// refused. An all-optional struct decodes `{}` like any real edit, so the
// handler asks httperr.RequireAnyField, or sits in emptyBodyAccepted.

import (
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// emptyBodyAccepted are the operations whose handler does not refuse `{}`.
var emptyBodyAccepted = gatekit.Waive(map[string]string{})

func TestEveryBodyThatBoundsItsPropertiesRefusesAnEmptyObject(t *testing.T) {
	t.Parallel()
	defer emptyBodyAccepted.AssertAllMatched(t)
	doc := loadContractDocument(t)
	schemas := descendContract(t, doc, "components", "schemas")
	ops := minPropertiesOperations(t, doc, schemas)
	for _, handler := range []string{"UpdateRole", "UpdateWebhookSubscription", "UpdateVoiceProfile"} {
		found := false
		for _, op := range ops {
			found = found || op.handler == handler
		}
		if !found {
			t.Fatalf("the census found %d operation(s) and not %s; the walk lost its subjects", len(ops), handler)
		}
	}
	handlers := handlerCalls(t)
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
			"Call httperr.RequireAnyField after the decode, or record why the handler is exempt.",
			len(missing), strings.Join(missing, "\n\t"))
	}
}

type minPropertiesOperation struct{ method, path, handler string }

// minPropertiesOperations lists each operation whose JSON body schema carries
// `minProperties`, with the handler method the generated router calls.
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
			if schema == nil || resolveSchema(schemas, schema)["minProperties"] == nil {
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
