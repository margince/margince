// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Each tool's arguments and the runner's step schema go through the Gemini
// rewrite. The decoder answers every branch in turn. It fills an optional object
// one level down and a list with one item, and nulls every other padded key.
// The undo must hand on a value valid under the schema sent, clearing nothing.
func TestEveryToolsArgumentsSurviveTheGeminiStrictWire(t *testing.T) {
	t.Parallel()
	schemas := wireSchemas(t)
	padded, branched := 0, 0
	for name, original := range schemas {
		strict, err := ai.GeminiStrictSchema(original)
		if err != nil {
			t.Errorf("%s: the strict wire cannot read its schema: %v", name, err)
			continue
		}
		var sent map[string]any
		if err := json.Unmarshal(strict, &sent); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(strict), `{"type":"null"}`) {
			padded++
		}
		branches := max(1, len(nonNullBranches(sent)))
		if branches > 1 {
			branched++
		}
		for choice := range branches {
			decoded, err := json.Marshal(leastValue(sent, choice, 0))
			if err != nil {
				t.Fatal(err)
			}
			handed := ai.GeminiDropPaddedNulls(original, string(decoded))
			if err := validJSON(original, handed); err != nil {
				t.Errorf("%s, branch %d: the value handed on breaks the schema sent: %v\n%s", name, choice, err, handed)
			}
			for _, key := range clearableKeys(t, original) {
				if strings.Contains(handed, `"`+key+`":null`) {
					t.Errorf("%s: %s, which the model may clear, came back null though nobody asked to clear it", name, key)
				}
			}
		}
	}
	if padded == 0 || branched == 0 {
		t.Fatalf("%d schemas were padded and %d had branches, so the undo was not exercised", padded, branched)
	}
}

// wireSchemas holds the schemas a Gemini tool call is sent under: each
// registered tool's arguments, and the runner's step schemas over them. Each
// caller gets its own copy of the one set built per test binary.
func wireSchemas(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	builtWireSchemas.Lock()
	defer builtWireSchemas.Unlock()
	if builtWireSchemas.schemas == nil {
		builtWireSchemas.schemas = buildWireSchemas(t)
	}
	schemas := make(map[string]json.RawMessage, len(builtWireSchemas.schemas))
	for name, schema := range builtWireSchemas.schemas {
		schemas[name] = bytes.Clone(schema)
	}
	return schemas
}

// builtWireSchemas stays nil when a build fails, so the next caller fails the
// same way.
var builtWireSchemas struct {
	sync.Mutex
	schemas map[string]json.RawMessage
}

func buildWireSchemas(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	registry := compose.NewRegistry(nil, compose.SendPath{})
	specs := registry.Specs()
	if len(specs) < 100 {
		t.Fatalf("the registry offered %d tools; the census lost its subject", len(specs))
	}
	schemas := stepSchemas(t, registry)
	for _, spec := range specs {
		schemas[spec.Name] = spec.InputSchema
	}
	return schemas
}

// The strict wire's undo cannot round-trip two shapes, and no schema sent
// today has either. A schema that adds one fails here, naming it, rather than
// reaching Gemini:
//
//   - a key declared both on a node and in one of its anyOf or oneOf branches,
//     nullable in one and not the other, so the padding is read two ways;
//   - an anyOf or oneOf branch that requires keys but declares no properties,
//     beside padded properties of the node, which the branch cannot match.
//
// The round-trip gate below also fills a node's own properties beside its
// branch. No schema sent today declares both, so removing the node-level walk
// from the undo leaves that gate green; TestGeminiUndoesANodesOwnPaddingBesideItsBranch
// in modules/ai holds it instead.
func TestNoWireSchemaHasAShapeTheStrictUndoCannotRoundTrip(t *testing.T) {
	t.Parallel()
	for name, raw := range wireSchemas(t) {
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		for _, found := range strictWireMisfits(schema, "$") {
			t.Errorf("%s: %s; geministrict cannot round-trip it, so change the schema or teach the undo first", name, found)
		}
	}
}

// Each shape the census names is found where it is, and a schema without it
// passes.
func TestTheStrictWireMisfitCensusFindsEachShape(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		schema string
		want   string
	}{
		"a key nullable on the node and not in the branch": {
			`{"type":"object","properties":{"note":{"type":["string","null"]}},"anyOf":[{"type":"object","properties":{"note":{"type":"string"}}}]}`,
			"note",
		},
		"a branch that requires keys and declares none": {
			`{"type":"object","properties":{"note":{"type":"string"}},"anyOf":[{"required":["id"]}]}`,
			"requires keys",
		},
		"either shape one level down": {
			`{"type":"object","properties":{"args":{"type":"object","properties":{"note":{"type":"string"}},"oneOf":[{"required":["id"]}]}}}`,
			"$.properties.args",
		},
		"neither shape": {
			`{"type":"object","properties":{"note":{"type":"string"}},"anyOf":[{"type":"object","properties":{"note":{"type":"string"}},"required":["note"]}]}`,
			"",
		},
	} {
		var schema map[string]any
		if err := json.Unmarshal([]byte(tc.schema), &schema); err != nil {
			t.Fatal(err)
		}
		found := strings.Join(strictWireMisfits(schema, "$"), "; ")
		if (tc.want == "") != (found == "") || !strings.Contains(found, tc.want) {
			t.Errorf("%s: found %q, want one naming %q", name, found, tc.want)
		}
	}
}

// strictWireMisfits names, by path, each place in schema with a shape the
// strict undo cannot round-trip, at every depth a schema nests another.
func strictWireMisfits(schema map[string]any, path string) []string {
	var found []string
	props, _ := schema["properties"].(map[string]any)
	required := stringsOf(schema["required"])
	paddedOwn := false
	for key, prop := range props {
		node, _ := prop.(map[string]any)
		paddedOwn = paddedOwn || (!slices.Contains(required, key) && !nullable(node))
	}
	for i, branch := range branchesOf(schema) {
		branchProps, declared := branch["properties"].(map[string]any)
		if !declared && len(stringsOf(branch["required"])) > 0 && paddedOwn {
			found = append(found, fmt.Sprintf("%s: branch %d requires keys but declares no properties beside padded ones", path, i))
		}
		for key, own := range props {
			ownNode, _ := own.(map[string]any)
			theirs, ok := branchProps[key].(map[string]any)
			if ok && nullable(ownNode) != nullable(theirs) {
				found = append(found, fmt.Sprintf("%s: %s is nullable on the node or in branch %d and not in the other", path, key, i))
			}
		}
	}
	for key, prop := range props {
		if node, ok := prop.(map[string]any); ok {
			found = append(found, strictWireMisfits(node, path+".properties."+key)...)
		}
	}
	for _, key := range []string{"items", "additionalProperties"} {
		if node, ok := schema[key].(map[string]any); ok {
			found = append(found, strictWireMisfits(node, path+"."+key)...)
		}
	}
	for i, branch := range branchesOf(schema) {
		found = append(found, strictWireMisfits(branch, fmt.Sprintf("%s.branch%d", path, i))...)
	}
	return found
}

// branchesOf are a schema's anyOf, oneOf and allOf members that are objects.
func branchesOf(schema map[string]any) []map[string]any {
	var out []map[string]any
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		members, _ := schema[key].([]any)
		for _, member := range members {
			if node, ok := member.(map[string]any); ok {
				out = append(out, node)
			}
		}
	}
	return out
}

// stepSchemas are the runner's own step schemas, captured at the model
// boundary, for the registry's tools in groups the window carries.
func stepSchemas(t *testing.T, registry *agents.Registry) map[string]json.RawMessage {
	t.Helper()
	var scopes []principal.Scope
	for _, spec := range registry.Specs() {
		scopes = append(scopes, spec.RequiredScope)
	}
	user := ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:wire", UserID: user, OnBehalfOf: user,
		PassportID: ids.NewV7(), SeatType: principal.SeatFull, Scopes: principal.NewScopeSet(scopes...),
	})
	var names []string
	for _, spec := range registry.Runnable(ctx) {
		names = append(names, spec.Name)
	}
	slices.Sort(names)
	out := map[string]json.RawMessage{}
	for group := range slices.Chunk(names, 8) {
		brain := &stepRecorder{}
		if _, err := runner.New(registry, brain).Run(ctx, runner.Job{Goal: "sweep", Tools: group}); err != nil {
			t.Fatalf("run over %v: %v", group, err)
		}
		if brain.schema == nil {
			t.Fatalf("the run over %v made no model call", group)
		}
		out["step:"+strings.Join(group, ",")] = brain.schema
	}
	if len(out) == 0 {
		t.Fatal("no step schema was captured")
	}
	return out
}

// stepRecorder is the model boundary: it keeps the first step schema and ends the run.
type stepRecorder struct{ schema json.RawMessage }

func (r *stepRecorder) Complete(_ context.Context, req model.Request) (model.Response, runner.Meta, error) {
	if r.schema == nil {
		r.schema = req.ResponseSchema
	}
	return model.Response{Text: `{"final":{"summary":"done"}}`}, runner.Meta{}, nil
}

func (*stepRecorder) PromptWindow() int { return 0 }

// leastValue is what a constrained decoder writes under schema. It takes the
// choice-th branch and every required key. It fills an optional object or list
// one level down and nulls every other key that admits it. Elsewhere it writes
// the least value allowed.
//
//craft:ignore naked-any a JSON schema decoded generically holds values of any JSON type, and so does the value it describes
func leastValue(schema map[string]any, choice, depth int) any {
	if branches := nonNullBranches(schema); len(branches) > 0 {
		value := leastValue(branches[choice%len(branches)], choice, depth)
		branchObject, isObject := value.(map[string]any)
		if _, own := schema["properties"]; !own || !isObject {
			return value
		}
		// The node's own properties are written beside the branch's.
		out := leastObject(schema, choice, depth)
		maps.Copy(out, branchObject)
		return out
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return values[0]
	}
	switch firstType(schema) {
	case "object":
		return leastObject(schema, choice, depth)
	case "array":
		items, _ := schema["items"].(map[string]any)
		least, _ := schema["minItems"].(float64)
		out := make([]any, max(1, int(least)))
		for i := range out {
			out[i] = leastValue(items, choice, depth+1)
		}
		return out
	case "string":
		least, _ := schema["minLength"].(float64)
		return strings.Repeat("x", max(1, int(least)))
	case "integer", "number":
		least, _ := schema["minimum"].(float64)
		return least
	case "boolean":
		return false
	default:
		return nil
	}
}

func leastObject(schema map[string]any, choice, depth int) map[string]any {
	out := map[string]any{}
	props, _ := schema["properties"].(map[string]any)
	// A conditional requirement is the decoder's to meet too: the least value
	// is the one its own enums put on the if's side.
	branch := "else"
	if condition, ok := schema["if"].(map[string]any); ok && conditionHolds(condition, props) {
		branch = "then"
	}
	var conditional []string
	if extra, ok := schema[branch].(map[string]any); ok {
		conditional = stringsOf(extra["required"])
	}
	for _, key := range append(stringsOf(schema["required"]), conditional...) {
		prop, _ := props[key].(map[string]any)
		switch {
		case prop == nil:
			out[key] = nil
		case slices.Contains(conditional, key) || (depth == 0 && nullable(prop) && nested(withoutNull(prop))):
			out[key] = leastValue(withoutNull(prop), choice, depth+1)
		case nullable(prop):
			out[key] = nil
		default:
			out[key] = leastValue(prop, choice, depth+1)
		}
	}
	return out
}

// conditionHolds says an if-schema's enums admit the least value of each
// property it names, which is the value leastValue writes.
func conditionHolds(condition, props map[string]any) bool {
	named, _ := condition["properties"].(map[string]any)
	for key, rule := range named {
		ruleNode, _ := rule.(map[string]any)
		prop, _ := props[key].(map[string]any)
		values, _ := ruleNode["enum"].([]any)
		if len(values) > 0 && prop != nil && !slices.Contains(values, leastValue(prop, 0, 1)) {
			return false
		}
	}
	return true
}

// nonNullBranches are a schema's anyOf or oneOf branches that are not null.
func nonNullBranches(schema map[string]any) []map[string]any {
	var out []map[string]any
	for _, key := range []string{"anyOf", "oneOf"} {
		branches, _ := schema[key].([]any)
		for _, branch := range branches {
			if node, ok := branch.(map[string]any); ok && (firstType(node) != "" || !nullable(node)) {
				out = append(out, node)
			}
		}
	}
	return out
}

// withoutNull is the non-null side of a padded property.
func withoutNull(schema map[string]any) map[string]any {
	if branches := nonNullBranches(schema); len(branches) == 1 && nullable(schema) {
		return branches[0]
	}
	return schema
}

func nested(schema map[string]any) bool {
	kind := firstType(schema)
	return kind == "object" || kind == "array"
}

func nullable(schema map[string]any) bool {
	if slices.Contains(stringsOf(schema["type"]), "null") {
		return true
	}
	branches, _ := schema["anyOf"].([]any)
	return slices.ContainsFunc(branches, func(branch any) bool {
		node, ok := branch.(map[string]any)
		return ok && nullable(node)
	})
}

func firstType(schema map[string]any) string {
	for _, name := range stringsOf(schema["type"]) {
		if name != "null" {
			return name
		}
	}
	if _, ok := schema["properties"]; ok {
		return "object"
	}
	return ""
}

//craft:ignore naked-any a schema's type and required members are a string or a list, decoded generically
func stringsOf(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(string); ok {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}

// clearableKeys are the root properties a tool leaves optional and lets the
// model set to null. An absent key keeps such a field, and a null clears it.
func clearableKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	props, _ := schema["properties"].(map[string]any)
	required := stringsOf(schema["required"])
	var keys []string
	for key, prop := range props {
		node, ok := prop.(map[string]any)
		if ok && nullable(node) && !slices.Contains(required, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

func validJSON(schema json.RawMessage, text string) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("tool.json", doc); err != nil {
		return err
	}
	compiled, err := compiler.Compile("tool.json")
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
	if err != nil {
		return err
	}
	return compiled.Validate(instance)
}
