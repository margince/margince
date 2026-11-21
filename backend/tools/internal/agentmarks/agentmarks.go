// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agentmarks reads the two markers a request body member carries for
// agents. The skill and the action catalog both narrow a body through Narrow.
// So both offer an agent one shape, the shape the agent gate enforces.
package agentmarks

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

const (
	// HumanOnly flags a body member a human alone may set. The store refuses
	// any other principal; the agent is not offered the member at all.
	HumanOnly = "x-human-only"
	// AgentValues lists the values of an enum body member an agent may send.
	// The agent is offered those alone, and the agent gate refuses any other.
	AgentValues = "x-agent-values"
)

const schemaPrefix = "#/components/schemas/"

// combinators are the keys Narrow reads as one plain schema, in the order the
// action catalog's walk reads them.
var combinators = []string{"allOf", "oneOf", "anyOf"}

// Marks are what a body's markers tell the agent gate. HumanOnly names the
// members a human alone sets, sorted; AgentValues the values an agent may send.
type Marks struct {
	HumanOnly   []string
	AgentValues map[string][]string
}

// Narrow is body as an agent may send it, and the marks the agent gate enforces.
// A body with no marker comes back as given. A marked one comes back as one flat
// object: human-only members gone, each agent-valued enum cut to its values.
// Neither body nor schemas, components.schemas as decoded YAML, are changed.
//
// A marker the gate cannot hold is an error. The gate reads top-level members,
// and a marked enum must hold the values it names.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func Narrow(body any, schemas map[string]any) (any, Marks, error) {
	r := resolver{schemas: schemas}
	if !r.carries(body, map[string]bool{}) {
		return body, Marks{}, nil
	}
	top, isMap := body.(map[string]any)
	if !isMap {
		return nil, Marks{}, errors.New("a marked request body is not an object schema")
	}
	flat, err := r.flat(top, map[string]bool{})
	if err != nil {
		return nil, Marks{}, err
	}
	marks, err := r.narrowMembers(flat)
	if err != nil {
		return nil, Marks{}, err
	}
	for key, value := range flat {
		if key != "properties" && r.carries(value, map[string]bool{}) {
			return nil, Marks{}, fmt.Errorf("%s and %s are read on a top-level body member only", HumanOnly, AgentValues)
		}
	}
	return flat, marks, nil
}

// narrowMembers applies each top-level member's marker to flat in place and
// names what it applied.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (r resolver) narrowMembers(flat map[string]any) (Marks, error) {
	marks := Marks{AgentValues: map[string][]string{}}
	props, _ := flat["properties"].(map[string]any)
	required, _ := flat["required"].([]any)
	kept := make(map[string]any, len(props))
	for name, member := range props {
		fields, isMap := member.(map[string]any)
		if !isMap {
			kept[name] = member
			continue
		}
		resolved, err := r.flat(fields, map[string]bool{})
		if err != nil {
			return Marks{}, err
		}
		switch {
		case resolved[HumanOnly] != nil:
			if resolved[HumanOnly] != true {
				return Marks{}, fmt.Errorf("body member %s: %s is true or absent", name, HumanOnly)
			}
			if slices.Contains(required, any(name)) {
				return Marks{}, fmt.Errorf("body member %s is human-only and required, so no agent call could be complete", name)
			}
			marks.HumanOnly = append(marks.HumanOnly, name)
			continue
		case resolved[AgentValues] != nil:
			allowed, err := agentValues(name, resolved)
			if err != nil {
				return Marks{}, err
			}
			delete(resolved, AgentValues)
			resolved["enum"] = toAny(allowed)
			marks.AgentValues[name] = allowed
			member = resolved
		}
		if r.carries(resolved, map[string]bool{}) {
			return Marks{}, fmt.Errorf("body member %s: %s and %s are read on a top-level body member only", name, HumanOnly, AgentValues)
		}
		kept[name] = member
	}
	if props != nil {
		flat["properties"] = kept
	}
	sort.Strings(marks.HumanOnly)
	return marks, nil
}

// agentValues reads one member's marker against its own enum.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func agentValues(name string, fields map[string]any) ([]string, error) {
	listed, isList := fields[AgentValues].([]any)
	enum, _ := fields["enum"].([]any)
	if !isList || len(listed) == 0 || len(enum) == 0 {
		return nil, fmt.Errorf("body member %s: %s lists values of the member's enum", name, AgentValues)
	}
	allowed := make([]string, 0, len(listed))
	for _, value := range listed {
		word, isWord := value.(string)
		if !isWord || !slices.Contains(enum, any(word)) {
			return nil, fmt.Errorf("body member %s: %s names %v, which its enum %v does not hold", name, AgentValues, value, enum)
		}
		allowed = append(allowed, word)
	}
	return allowed, nil
}

// resolver reads $refs against components.schemas.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
type resolver struct{ schemas map[string]any }

//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (r resolver) target(ref string) (map[string]any, error) {
	target, known := r.schemas[strings.TrimPrefix(ref, schemaPrefix)].(map[string]any)
	if !strings.HasPrefix(ref, schemaPrefix) || !known {
		return nil, fmt.Errorf("body names schema %s, which components.schemas does not declare", ref)
	}
	return target, nil
}

// flat is a fresh copy of node's top with its $ref and combinators resolved. A
// key beside a $ref wins over the target's: it is the member's own word. allOf
// merges its members; oneOf and anyOf keep the first that is not null.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (r resolver) flat(node map[string]any, open map[string]bool) (map[string]any, error) {
	if ref, isRef := node["$ref"].(string); isRef {
		if open[ref] {
			return nil, fmt.Errorf("schema %s names itself at its own top", ref)
		}
		target, err := r.target(ref)
		if err != nil {
			return nil, err
		}
		open[ref] = true
		defer delete(open, ref)
		out, err := r.flat(target, open)
		if err != nil {
			return nil, err
		}
		for key, value := range node {
			if key != "$ref" {
				out[key] = value
			}
		}
		return out, nil
	}
	for _, key := range combinators {
		if members, isList := node[key].([]any); isList {
			return r.merged(node, key, members, open)
		}
	}
	return maps.Clone(node), nil
}

// merged reads one combinator as a plain schema. A member it leaves out must
// carry no marker, since no agent would ever be held to it.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (r resolver) merged(node map[string]any, key string, members []any, open map[string]bool) (map[string]any, error) {
	out := maps.Clone(node)
	delete(out, key)
	taken := false
	for _, member := range members {
		fields, isMap := member.(map[string]any)
		if !isMap || fields["type"] == "null" {
			continue
		}
		if taken && key != "allOf" {
			if r.carries(fields, map[string]bool{}) {
				return nil, fmt.Errorf("a marker sits in a %s member past the first, which an agent is never offered", key)
			}
			continue
		}
		resolved, err := r.flat(fields, open)
		if err != nil {
			return nil, err
		}
		mergeInto(out, resolved)
		taken = true
	}
	return r.flat(out, open)
}

// mergeInto adds one resolved schema's members to another, uniting their
// properties and required lists.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func mergeInto(into, member map[string]any) {
	for key, value := range member {
		switch key {
		case "properties":
			props := map[string]any{}
			if have, ok := into[key].(map[string]any); ok {
				maps.Copy(props, have)
			}
			if more, ok := value.(map[string]any); ok {
				maps.Copy(props, more)
			}
			into[key] = props
		case "required":
			have, _ := into[key].([]any)
			more, _ := value.([]any)
			into[key] = append(slices.Clone(have), more...)
		default:
			into[key] = value
		}
	}
}

// carries reports a marker anywhere under node, following each $ref once.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (r resolver) carries(node any, seen map[string]bool) bool {
	switch v := node.(type) {
	case map[string]any:
		if v[HumanOnly] != nil || v[AgentValues] != nil {
			return true
		}
		if ref, isRef := v["$ref"].(string); isRef && !seen[ref] {
			seen[ref] = true
			if target, err := r.target(ref); err == nil && r.carries(target, seen) {
				return true
			}
		}
		for _, member := range v {
			if r.carries(member, seen) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, func(member any) bool { return r.carries(member, seen) })
	}
	return false
}

//craft:ignore naked-any an enum is written back as decoded YAML, whatever its values
func toAny(words []string) []any {
	out := make([]any, len(words))
	for i, word := range words {
		out[i] = word
	}
	return out
}
