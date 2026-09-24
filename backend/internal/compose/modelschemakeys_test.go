// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

// The keyed-object gate: no schema this build hands a model declares an object
// without naming its keys.
//
// Gemini's constrained decoder admits no key into an object whose schema lists
// no properties, and `additionalProperties` does not reopen it. The agent loop
// learned this at a cost: a model that wanted to write tool arguments padded
// `"args": {` with whitespace to the output ceiling. Every object in a
// ResponseSchema and in the input schema of a tool a model is offered is
// therefore held to declaring its keys. The one legitimate empty object is a
// tool that takes no arguments: `{"type":"object","properties":{}}`, where an
// empty object is the whole answer.

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// unkeyedToolsNoAgentIsGiven waives, per tool and object path, the served input
// schemas that still carry an object without declared keys, for one reason: the
// object is a caller-shaped map whose keys are the WORKSPACE's (a record's own
// fields, a query plan, a column mapping), so declaring them is a product
// decision about the tool's contract rather than a schema fix. No scheduled
// agent is given one, which is the only way this build hands one to a model
// under constrained decoding; MCP clients call them with their own decoders.
//
// SHRINK-ONLY: an entry whose tool is attached to an agent, is unregistered, or
// whose object is now keyed fails TestEveryServedToolNamesTheKeysOfItsObjects.
var unkeyedToolsNoAgentIsGiven = gatekit.Waive(map[string]string{
	"create_record $.fields":   "fields is keyed by the record type's own field names",
	"update_record $.fields":   "fields is keyed by the record type's own field names",
	"query_workspace $.plan":   "plan is the query grammar's own open document",
	"run_report $.filters":     "filters is keyed by the report source's own dimensions",
	"preview_import $.mapping": "mapping is keyed by the uploaded file's own column headers",
})

func TestNoRequestAModelIsSentCarriesAnObjectWithoutKeys(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	bySite := map[string][]aicert.Scenario{}
	for _, sc := range scenarios {
		bySite[sc.Task+"/"+sc.Site] = append(bySite[sc.Task+"/"+sc.Site], sc)
	}
	schemaless := publishedSchemalessSites(t)
	zeroArgument := zeroArgumentTools(servedSpecs())

	for _, site := range census.All() {
		key := string(site.Task) + "/" + site.Variant
		t.Run(key, func(t *testing.T) {
			owned := bySite[key]
			if len(owned) == 0 {
				t.Fatalf("site %s has no corpus scenario, so no request of its is ever inspected", key)
			}
			schemas := 0
			for _, sc := range owned {
				for i, req := range issuedRequests(t, census, sc) {
					if len(req.ResponseSchema) == 0 {
						continue
					}
					schemas++
					for _, defect := range unkeyedObjects(t, req.ResponseSchema, zeroArgument, false) {
						t.Errorf("scenario %s, request %d: %s%s", sc.Name, i+1, defect, declaresNoKeys)
					}
				}
			}
			// A case that stops early on the stand-in reply can issue none of
			// the requests that carry a schema, and would pass having read none.
			if schemas == 0 && !schemaless[key] {
				t.Errorf("site %s sent no ResponseSchema, and docs/reference/ai-prompts.json does not publish it "+
					"as schemaless — either its case stopped before the constrained request, or the page is stale", key)
			}
		})
	}
}

// issuedRequests drives one scenario's real case and returns every request it
// issued. A refusal of the stand-in reply is the case's validator working; the
// requests already issued are what is inspected.
func issuedRequests(t *testing.T, census *aitasks.Registry, sc aicert.Scenario) []model.Request {
	t.Helper()
	factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !bound {
		t.Fatalf("scenario %s names site %s/%s, which no case is bound to", sc.Name, sc.Task, sc.Site)
	}
	prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if err != nil {
		t.Fatalf("scenario %s: the site refused its own committed fixture: %v", sc.Name, err)
	}
	completer := &recordingCompleter{reply: canaryReply}
	if _, runErr := prepared.Run(t.Context(), completer); runErr != nil {
		t.Logf("scenario %s: the case refused the stand-in reply (%v); inspecting the %d request(s) it issued",
			sc.Name, runErr, len(completer.requests))
	}
	if len(completer.requests) == 0 {
		t.Fatalf("scenario %s issued no request, so nothing it sends was inspected", sc.Name)
	}
	return completer.requests
}

// publishedSchemalessSites is the sites the published prompts record lists with
// no answer schema — the declaration a site that yields none is held to. It is
// read off the wire by TestTheAIPromptsPageIsCurrent, never written by hand.
func publishedSchemalessSites(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(aiPromptsDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", aiPromptsDoc, err)
	}
	var doc promptDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", aiPromptsDoc, err)
	}
	schemaless := map[string]bool{}
	for _, site := range doc.Sites {
		if len(site.AnswerSchema) == 0 {
			schemaless[site.Task+"/"+site.Site] = true
		}
	}
	return schemaless
}

func TestEveryServedToolNamesTheKeysOfItsObjects(t *testing.T) {
	specs := servedSpecs()
	zeroArgument := zeroArgumentTools(specs)
	attached := attachedTools(t)

	for _, spec := range specs {
		for _, defect := range unkeyedObjects(t, spec.InputSchema, zeroArgument, zeroArgument[spec.Name]) {
			offence := spec.Name + " " + defect
			if !unkeyedToolsNoAgentIsGiven.Waived(t, offence) {
				t.Errorf("tool %s: %s%s", spec.Name, defect, declaresNoKeys)
				continue
			}
			if attached[spec.Name] {
				t.Errorf("%s is attached to a scheduled agent while %s is waived — a constrained decoder "+
					"cannot fill that object: declare its keys before attaching the tool", spec.Name, defect)
			}
		}
	}
	// A waived object that was keyed, or whose tool was unregistered, is no
	// longer asked about, so the entry is reported stale and the list only shrinks.
	unkeyedToolsNoAgentIsGiven.AssertAllMatched(t)
}

// declaresNoKeys finishes every finding: what the defect costs a model.
const declaresNoKeys = " is an object that declares no keys — a constrained decoder can write none into it"

// The gate recognises every shape of the defect it names, and spares the one
// legitimate empty object only where a zero-argument tool is named.
func TestTheKeyedObjectGateSeesEveryUnkeyedShape(t *testing.T) {
	zeroArgument := map[string]bool{"whoami": true}
	for name, tc := range map[string]struct {
		schema  string
		unkeyed bool
	}{
		"a bare object":                                {`{"type":"object"}`, true},
		"keys only from additionalProperties":          {`{"type":"object","additionalProperties":{"type":"string"}}`, true},
		"nested under properties":                      {`{"type":"object","properties":{"f":{"type":"object"}}}`, true},
		"nested under items":                           {`{"type":"array","items":{"type":"object"}}`, true},
		"nested under anyOf":                           {`{"anyOf":[{"type":"object","properties":{"a":{"type":"string"}}},{"type":"object"}]}`, true},
		"a nullable object":                            {`{"type":["object","null"]}`, true},
		"an empty object outside a zero-argument tool": {`{"anyOf":[{"type":"object","properties":{"tool":{"type":"string","enum":["read_record"]},"args":{"type":"object","properties":{}}}}]}`, true},
		"a keyed object":                               {`{"type":"object","properties":{"a":{"type":"string"}}}`, false},
		"a zero-argument tool's args":                  {`{"anyOf":[{"type":"object","properties":{"tool":{"type":"string","enum":["whoami"]},"args":{"type":"object","properties":{}}}}]}`, false},
		"an enum holding an object value":              {`{"type":"string","enum":[{"type":"object"}]}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := len(unkeyedObjects(t, json.RawMessage(tc.schema), zeroArgument, false)) > 0; got != tc.unkeyed {
				t.Errorf("unkeyed = %v, want %v for %s", got, tc.unkeyed, tc.schema)
			}
		})
	}
}

func servedSpecs() []mcp.ToolSpec {
	return compose.NewRegistry(nil, compose.SendPath{}).Specs()
}

// zeroArgumentTools names the served tools whose input schema declares no keys
// at all — the one place an empty object is the honest shape.
func zeroArgumentTools(specs []mcp.ToolSpec) map[string]bool {
	zero := map[string]bool{}
	for _, spec := range specs {
		var declared struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(spec.InputSchema, &declared) == nil && declared.Properties != nil && len(declared.Properties) == 0 {
			zero[spec.Name] = true
		}
	}
	return zero
}

// attachedTools names the tools the scheduled agents are given, resolved the
// way the runner service resolves an agent.
func attachedTools(t *testing.T) map[string]bool {
	t.Helper()
	attached := map[string]bool{}
	for _, entry := range runner.Catalog() {
		agent, ok := compose.ScheduledAgentSpecByName(entry.Name)
		if !ok {
			t.Fatalf("scheduled agent %s resolves to no spec", entry.Name)
		}
		for _, tool := range agent.Tools {
			attached[tool] = true
		}
	}
	return attached
}

// unkeyedObjects returns the path of each object node in a schema that
// declares no keys.
//
// It walks EVERY keyword rather than a list of the ones that nest schemas, so a
// keyword nobody thought of widens the walk instead of hiding a subtree; only
// the keywords that hold values or names rather than schemas are skipped.
// emptyAllowed spares `{"type":"object","properties":{}}` at the root, for a
// zero-argument tool's own input schema.
func unkeyedObjects(t *testing.T, raw json.RawMessage, zeroArgument map[string]bool, emptyAllowed bool) []string {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("the schema is not a JSON object: %v", err)
	}
	var defects []string
	walkSchema(root, "$", emptyAllowed, zeroArgument, &defects)
	sort.Strings(defects)
	return defects
}

// valueKeywords hold names, values or annotations — never a schema — so an
// object inside one (an enum member, a default) is data, not an object schema.
var valueKeywords = map[string]bool{
	"enum": true, "const": true, "default": true, "examples": true, "required": true,
	"type": true, "description": true, "title": true, "format": true, "pattern": true,
}

// nameKeyedKeywords map a caller's names to schemas: the keys are not keywords
// and the values are each walked.
var nameKeyedKeywords = map[string]bool{
	"properties": true, "patternProperties": true, "$defs": true, "definitions": true, "dependentSchemas": true,
}

//craft:ignore naked-any node is a decoded JSON Schema, whose keyword values are object, array or scalar by turns
func walkSchema(node map[string]any, path string, emptyAllowed bool, zeroArgument map[string]bool, defects *[]string) {
	properties, declares := node["properties"].(map[string]any)
	spared := emptyAllowed && declares
	if describesObject(node) && len(properties) == 0 && !spared {
		*defects = append(*defects, path)
	}
	argsExempt := zeroArgument[calledTool(properties)]
	for key, value := range node {
		switch {
		case valueKeywords[key]:
		case nameKeyedKeywords[key]:
			byName, isMap := value.(map[string]any)
			if !isMap {
				continue
			}
			for name, sub := range byName {
				if subschema, isSchema := sub.(map[string]any); isSchema {
					walkSchema(subschema, path+"."+name, key == "properties" && name == "args" && argsExempt, zeroArgument, defects)
				}
			}
		default:
			walkNested(value, path+"."+key, zeroArgument, defects)
		}
	}
}

//craft:ignore naked-any value is one decoded keyword value: a schema, a list of schemas, or a scalar
func walkNested(value any, path string, zeroArgument map[string]bool, defects *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		walkSchema(typed, path, false, zeroArgument, defects)
	case []any:
		for i, item := range typed {
			walkNested(item, fmt.Sprintf("%s[%d]", path, i), zeroArgument, defects)
		}
	}
}

// describesObject reports whether a node is an object schema, by any spelling:
// `type: object`, a union including it, or keys given only through
// additionalProperties.
//
//craft:ignore naked-any node is a decoded JSON Schema
func describesObject(node map[string]any) bool {
	switch declared := node["type"].(type) {
	case string:
		if declared == "object" {
			return true
		}
	case []any:
		if slices.Contains(declared, any("object")) {
			return true
		}
	}
	_, extraSchema := node["additionalProperties"].(map[string]any)
	_, declaresProperties := node["properties"]
	return extraSchema || declaresProperties
}

// calledTool is the one tool a step branch names — `tool` as a single-member
// enum beside `args` — or "" for any other object.
//
//craft:ignore naked-any properties is a decoded JSON Schema properties map
func calledTool(properties map[string]any) string {
	if _, hasArgs := properties["args"]; !hasArgs {
		return ""
	}
	tool, isSchema := properties["tool"].(map[string]any)
	if !isSchema {
		return ""
	}
	names, isList := tool["enum"].([]any)
	if !isList || len(names) != 1 {
		return ""
	}
	if name, isName := names[0].(string); isName {
		return name
	}
	return ""
}
