// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A top-level argument's declared JSON `type`, held at the chokepoint numargs.go
// uses. It runs before a call is staged, so no human approves a call its handler
// would refuse. An extension handler checks types only after the approval is spent.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// typedArg is one argument's declared JSON type. A schema type is a single word
// here, because the bounds reader already refuses any other spelling at boot.
type typedArg struct {
	name string
	kind string
}

// declaredTypes reads a schema's top-level property types once, at registration.
// A property with no `type` stays unchecked.
func declaredTypes(inputSchema json.RawMessage) []typedArg {
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if json.Unmarshal(inputSchema, &schema) != nil {
		return nil
	}
	var out []typedArg
	for name, prop := range schema.Properties {
		if prop.Type != "" {
			out = append(out, typedArg{name: name, kind: prop.Type})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// requireDeclaredTypes refuses a present, non-null argument of a type its
// schema does not declare.
func (r *Registry) requireDeclaredTypes(name string, args json.RawMessage) error {
	r.mu.RLock()
	typed := r.typedArgs[name]
	r.mu.RUnlock()
	present, isObject := argsAsObject(args)
	if len(typed) == 0 || !isObject {
		return nil
	}
	var refusals, fields []string
	for _, arg := range typed {
		raw, supplied := present[arg.name]
		got := jsonType(raw)
		if !supplied || got == "null" || got == arg.kind || got == "integer" && arg.kind == "number" {
			continue
		}
		refusals = append(refusals, fmt.Sprintf("`%s` is %s, and its input schema declares %s", arg.name, got, arg.kind))
		fields = append(fields, arg.name)
	}
	if len(refusals) == 0 {
		return nil
	}
	bad := &BadArgsError{Cause: errors.New(strings.Join(refusals, "; "))}
	if len(fields) == 1 {
		bad.Field = fields[0]
	}
	return bad
}

// jsonType names a value's JSON type in schema words. A whole number is an
// integer, which a schema declaring "number" also accepts.
func jsonType(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "invalid"
	}
	switch typed := value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case float64:
		if typed == float64(int64(typed)) {
			return "integer"
		}
		return "number"
	}
	return "null"
}
