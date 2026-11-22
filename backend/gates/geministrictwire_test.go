// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Every registered tool's arguments are rewritten as the Gemini wire sends
// them. A constrained decoder writes every required key, with null wherever the
// key admits one, and the wire's undo takes the padding back out. What comes
// back must be valid under the tool's own schema. A property the tool lets the
// model clear is never written for it. A rewrite the walk does not
// undo fails here.
func TestEveryToolsArgumentsSurviveTheGeminiStrictWire(t *testing.T) {
	t.Parallel()
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	if len(specs) < 100 {
		t.Fatalf("the registry offered %d tools; the census lost its subject", len(specs))
	}
	padded := 0
	for _, spec := range specs {
		strict, err := ai.GeminiStrictSchema(spec.InputSchema)
		if err != nil {
			t.Errorf("%s: the strict wire cannot read its schema: %v", spec.Name, err)
			continue
		}
		if strings.Contains(string(strict), `{"type":"null"}`) {
			padded++
		}
		var sent map[string]any
		if err := json.Unmarshal(strict, &sent); err != nil {
			t.Fatal(err)
		}
		decoded, err := json.Marshal(leastValue(sent))
		if err != nil {
			t.Fatal(err)
		}
		handed := ai.GeminiDropPaddedNulls(spec.InputSchema, string(decoded))
		if err := validJSON(spec.InputSchema, handed); err != nil {
			t.Errorf("%s: the arguments handed on break the tool's own schema: %v\n%s", spec.Name, err, handed)
		}
		for _, key := range clearableKeys(t, spec.InputSchema) {
			if strings.Contains(handed, `"`+key+`":null`) {
				t.Errorf("%s: %s, which the model may clear, came back null though nobody asked to clear it", spec.Name, key)
			}
		}
	}
	if padded == 0 {
		t.Fatal("no tool schema was padded, so no padding was taken back out")
	}
}

// leastValue is what a constrained decoder writes under schema: every required
// key, null wherever the key admits null, and the least value allowed elsewhere.
//
//craft:ignore naked-any a JSON schema decoded generically holds values of any JSON type, and so does the value it describes
func leastValue(schema map[string]any) any {
	if branches, ok := schema["anyOf"].([]any); ok && len(branches) > 0 {
		if branch, ok := branches[0].(map[string]any); ok {
			return leastValue(branch)
		}
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return values[0]
	}
	switch firstType(schema) {
	case "object":
		out := map[string]any{}
		props, _ := schema["properties"].(map[string]any)
		required := stringsOf(schema["required"])
		// A conditional requirement is the decoder's to meet too: the least
		// value is the one its own enums put on the if's side.
		branch := "else"
		if condition, ok := schema["if"].(map[string]any); ok && conditionHolds(condition, props) {
			branch = "then"
		}
		conditional := []string{}
		if extra, ok := schema[branch].(map[string]any); ok {
			conditional = stringsOf(extra["required"])
		}
		for _, key := range append(required, conditional...) {
			prop, _ := props[key].(map[string]any)
			if prop != nil && slices.Contains(conditional, key) {
				out[key] = leastValue(withoutNull(prop))
				continue
			}
			if prop == nil || nullable(prop) {
				out[key] = nil
				continue
			}
			out[key] = leastValue(prop)
		}
		return out
	case "array":
		items, _ := schema["items"].(map[string]any)
		least, _ := schema["minItems"].(float64)
		out := make([]any, int(least))
		for i := range out {
			out[i] = leastValue(items)
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

// conditionHolds says an if-schema's enums admit the least value of each
// property it names, which is the value leastValue writes.
func conditionHolds(condition, props map[string]any) bool {
	named, _ := condition["properties"].(map[string]any)
	for key, rule := range named {
		ruleNode, _ := rule.(map[string]any)
		prop, _ := props[key].(map[string]any)
		values, _ := ruleNode["enum"].([]any)
		if len(values) > 0 && prop != nil && !slices.Contains(values, leastValue(prop)) {
			return false
		}
	}
	return true
}

// withoutNull is the non-null side of a padded property, which a key the
// schema's own condition requires must take.
func withoutNull(schema map[string]any) map[string]any {
	branches, _ := schema["anyOf"].([]any)
	for _, branch := range branches {
		if node, ok := branch.(map[string]any); ok && !nullable(node) {
			return node
		}
	}
	return schema
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
