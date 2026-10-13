// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every route taking `If-Match` is checked for a read that publishes the
// record's version, with the routes having none ratified by name.
//
// Such a route conditions on a version, and a caller learns one by reading the
// record. So the schema that read answers has to publish it.
//
// Where it does not, the precondition is decoration and every step still
// works. The parameter is declared, the handler reads the header, and the store
// compares the number.
//
// The only call that can succeed is one omitting the header, so the guard
// refuses nothing and no test fails.
//
// `archivePipeline` shipped that way and was fixed in margince#5155.
//
// The rows this gate carries as debt were already here. Each owes the
// per-record call margince#5209 asks for: publish a version, or drop the
// header.

import (
	"maps"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	ifMatchParameter = "#/components/parameters/IfMatch"
	rowVersionSchema = "#/components/schemas/RowVersion"
)

// conditionalRoutesOwedAVersion ratifies the routes whose caller has no read to
// take a version from.
//
// Each answers no record at all. The path carries only a write verb, and
// walking up to the nearest GET finds nothing publishing a row version.
//
// Every entry owes the per-record call margince#5209 asks for, which is a
// product question rather than a derivation one. Publish a version on a read
// the caller can reach, or drop a header that refuses nothing.
//
// Keyed by the operation rather than by schema, because these have no schema.
// Keying them together under the empty name ratifies a class rather than three
// routes, and a fourth would join it unnoticed.
//
// Bidirectional, like every waiver here: give the route a versioned read, or
// take the header off, and the entry goes stale on its own.
var conditionalRoutesOwedAVersion = gatekit.Waive(map[string]string{
	"DELETE /record-grants/{id}":                                 "margince#5209 — a grant is listed, never read on its own, so no response carries its version",
	"PATCH /provider-connections/{provider}":                     "margince#5209 — the connection is read through a provider summary that publishes no version",
	"POST /records/{record_type}/{id}/claim":                     "margince#5209 — the claim acts on a record the route does not read back",
	"PATCH /voice-profiles/{id}/sources/{sourceId}":              "margince#5209 — a source is listed under its profile and never read on its own",
	"DELETE /voice-profiles/{id}/sources/{sourceId}":             "margince#5209 — as the PATCH beside it",
	"POST /voice-profiles/{id}/versions/{profileVersion}/apply":  "margince#5209 — a profile version's read publishes no row version of its own",
	"POST /voice-profiles/{id}/versions/{profileVersion}/reject": "margince#5209 — as the apply beside it",
})

func TestEveryConditionalRouteOperatesOnAVersionedRecord(t *testing.T) {
	t.Parallel()
	defer conditionalRoutesOwedAVersion.AssertAllMatched(t)
	doc := ifMatchContractDocument(t)
	conditional := conditionalOperations(t, doc)
	// A census that reads nothing agrees with everything. The contract carries
	// conditional routes, so an empty answer means the shape moved.
	if len(conditional) == 0 {
		t.Fatal("no operation declares If-Match — this census is reading a shape that is gone")
	}
	for _, operation := range conditional {
		path := operation[strings.Index(operation, " ")+1:]
		record := recordAnsweredBy(doc, path)
		// The schema the read answers, or the item it wraps, tried in that
		// order. A record publishing its own version is the answer.
		//
		// A deal carries one, and it also carries a single array of tags. A
		// name-blind unwrap takes that array for the record it conditions on.
		if publishesAVersion(doc, record) || publishesAVersion(doc, unwrapListItem(doc, record)) {
			continue
		}
		if conditionalRoutesOwedAVersion.Waived(t, operation) {
			continue
		}
		t.Errorf("%s declares If-Match and the record it conditions on (%s) publishes no version, "+
			"nor does the item it wraps: "+
			"a caller has no number to send, so the precondition refuses nothing. Publish "+
			"version: { $ref: %s } on that schema, or drop the header",
			operation, recordName(record), rowVersionSchema)
	}
}

// A path item's own parameters reach every operation under it, so the census
// has to read both. Held here rather than argued, because a census that misses
// an inherited header reports PASS on a route it never looked at.
func TestTheCensusReadsAnInheritedConditionalHeader(t *testing.T) {
	t.Parallel()
	inherited := map[string]any{
		"paths": map[string]any{
			"/widgets/{id}": map[string]any{
				"parameters": []any{map[string]any{"$ref": ifMatchParameter}},
				"patch":      map[string]any{},
				"get":        map[string]any{},
			},
		},
	}
	got := conditionalOperations(t, inherited)
	if len(got) != 2 {
		t.Fatalf("read %v from a path declaring If-Match, want both operations under it", got)
	}

	// The other direction: the path item's non-verb keys are not operations.
	named := map[string]any{
		"paths": map[string]any{
			"/widgets/{id}": map[string]any{
				"summary": "not an operation",
				"patch": map[string]any{
					"parameters": []any{map[string]any{"$ref": ifMatchParameter}},
				},
				"get": map[string]any{},
			},
		},
	}
	if got := conditionalOperations(t, named); len(got) != 1 || got[0] != "PATCH /widgets/{id}" {
		t.Errorf("read %v where one operation declares the header, want PATCH alone", got)
	}
}

// recordName reads an empty schema as a sentence rather than as "()".
func recordName(schema string) string {
	if schema == "" {
		return "no schema — the path answers no record"
	}
	return schema
}

// contractDocument parses the contract loosely: a path item carries parameters
// beside its verbs, which no fixed struct for an operation holds.
func ifMatchContractDocument(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	return doc
}

// conditionalOperations answers every operation declaring If-Match, as the
// method and path a reader would name it by.
//
// Per operation and not per path. A path may condition on one verb and not
// another, and a waiver names the one it ratifies.
func conditionalOperations(t *testing.T, doc map[string]any) []string {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("the contract declares no paths — this census is reading a shape that is gone")
	}
	var out []string
	for path, item := range paths {
		operations, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// A path item's own parameters are inherited by every operation under
		// it, so a route can take If-Match without naming it. Reading the
		// operation alone omits that route while the others keep the census
		// nonempty, which is the way this census could fail short.
		inherited, _ := operations["parameters"].([]any)
		for method, raw := range operations {
			if !isHTTPMethod(method) {
				continue
			}
			operation, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			own, _ := operation["parameters"].([]any)
			if declaresIfMatch(inherited) || declaresIfMatch(own) {
				out = append(out, strings.ToUpper(method)+" "+path)
			}
		}
	}
	return out
}

// isHTTPMethod separates an operation from the other keys a path item carries,
// `parameters` and `summary` among them.
func isHTTPMethod(key string) bool {
	switch key {
	case "get", "put", "post", "delete", "patch", "head", "options", "trace":
		return true
	}
	return false
}

// declaresIfMatch reads one operation's parameter list for the shared header.
func declaresIfMatch(parameters []any) bool {
	for _, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if ref, _ := parameter["$ref"].(string); ref == ifMatchParameter {
			return true
		}
	}
	return false
}

// recordAnsweredBy walks up from a conditional path to the nearest ancestor
// with a GET, and answers the schema that read returns. That read is where a
// caller gets the version, so it is the one that has to publish it.
//
// A list wrapper is unwrapped to its item. A fact under a company walks up to a
// GET answering a list of facts, and the version it conditions on is one
// fact's.
func recordAnsweredBy(doc map[string]any, path string) string {
	paths, _ := doc["paths"].(map[string]any)
	for candidate := path; candidate != ""; candidate = parentPath(candidate) {
		item, ok := paths[candidate].(map[string]any)
		if !ok {
			continue
		}
		get, ok := item["get"].(map[string]any)
		if !ok {
			continue
		}
		// The NEAREST read answers, whether or not this gate can follow its
		// schema. Walking past an inline response to a more distant GET takes
		// a version off a record the route does not condition on.
		return okResponseSchema(get)
	}
	return ""
}

// parentPath drops the last segment, so a verb under a record walks back to the
// record's own read.
func parentPath(path string) string {
	cut := strings.LastIndex(path, "/")
	if cut <= 0 {
		return ""
	}
	return path[:cut]
}

// okResponseSchema reads the schema name a GET's 200 answers, "" when it
// answers something this walk cannot follow.
func okResponseSchema(get map[string]any) string {
	responses, ok := get["responses"].(map[string]any)
	if !ok {
		return ""
	}
	ok200, found := responses["200"].(map[string]any)
	if !found {
		return ""
	}
	content, ok := ok200["content"].(map[string]any)
	if !ok {
		return ""
	}
	body, ok := content["application/json"].(map[string]any)
	if !ok {
		return ""
	}
	schema, ok := body["schema"].(map[string]any)
	if !ok {
		return ""
	}
	ref, _ := schema["$ref"].(string)
	return strings.TrimPrefix(ref, "#/components/schemas/")
}

// unwrapListItem answers the item schema of a list wrapper, or its argument
// where the schema is the record itself.
//
// The wrapper's array is found by SHAPE rather than by name. `data` is the
// common spelling and not the only one.
//
// The role directory wraps its items under `roles`, and judging the wrapper
// there reports a version the item publishes as missing.
//
// One array of records, or the wrapper stands. Two is a guess about which the
// route conditions on, so reporting the wrapper sends that ambiguity to a
// reader rather than resolving it here.
func unwrapListItem(doc map[string]any, schema string) string {
	properties := schemaProperties(doc, schema)
	// An envelope has no identity of its own, and a record does.
	//
	// So a record carrying one array of related records keeps its own missing
	// version as the finding. A company with an array of versioned contacts
	// must not pass on a contact's number.
	if _, hasID := properties["id"]; hasID {
		return schema
	}
	var items []string
	for _, property := range properties {
		field, ok := property.(map[string]any)
		if !ok {
			continue
		}
		element, ok := field["items"].(map[string]any)
		if !ok {
			continue
		}
		if ref, _ := element["$ref"].(string); ref != "" {
			items = append(items, strings.TrimPrefix(ref, "#/components/schemas/"))
		}
	}
	// One array of records, or the envelope stands. Two is a guess about which
	// the route conditions on, so reporting the envelope sends that ambiguity
	// to a reader rather than resolving it here.
	if len(items) != 1 {
		return schema
	}
	return items[0]
}

// publishesAVersion answers whether a schema carries the shared row version,
// following allOf so a composed schema is read through its parts.
func publishesAVersion(doc map[string]any, schema string) bool {
	if schema == "" {
		return false
	}
	// Named `version`, because that is the field a caller sends back. Another
	// property referencing RowVersion is some other record's number and gives
	// this caller nothing to put in the header.
	version, declared := schemaProperties(doc, schema)["version"].(map[string]any)
	if !declared {
		return false
	}
	if ref, _ := version["$ref"].(string); ref == rowVersionSchema {
		return true
	}
	// Spelled inline rather than through the shared ref, which still gives the
	// caller a number.
	kind, _ := version["type"].(string)
	return kind == "integer"
}

// schemaProperties reads one schema's properties, merging the parts of an
// allOf so a composed record reads as the whole it serves.
func schemaProperties(doc map[string]any, schema string) map[string]any {
	schemas, _ := doc["components"].(map[string]any)
	byName, _ := schemas["schemas"].(map[string]any)
	declared, ok := byName[schema].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if properties, ok := declared["properties"].(map[string]any); ok {
		maps.Copy(out, properties)
	}
	parts, _ := declared["allOf"].([]any)
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if ref, _ := part["$ref"].(string); ref != "" {
			maps.Copy(out, schemaProperties(doc, strings.TrimPrefix(ref, "#/components/schemas/")))
		}
		if properties, ok := part["properties"].(map[string]any); ok {
			maps.Copy(out, properties)
		}
	}
	return out
}
