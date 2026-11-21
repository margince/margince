// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"fmt"
	"slices"
)

// agentValuesMarker lists the values of an enum body member an agent may send.
// The agent is offered those alone, and the agent gate refuses any other.
const agentValuesMarker = "x-agent-values"

// withAgentValues narrows the enum of each top-level member that carries
// agentValuesMarker to the values listed, drops the marker, and returns the
// lists by member. A marker deeper down is refused, since the gate reads
// top-level members only, and so is one naming a value the enum does not hold.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func withAgentValues(root any) (map[string][]string, error) {
	narrowed := map[string][]string{}
	top, _ := root.(map[string]any)
	props, _ := top["properties"].(map[string]any)
	for name, member := range props {
		fields, ok := member.(map[string]any)
		if !ok || fields[agentValuesMarker] == nil {
			continue
		}
		allowed, err := agentValues(name, fields)
		if err != nil {
			return nil, err
		}
		fields["enum"] = toAny(allowed)
		delete(fields, agentValuesMarker)
		narrowed[name] = allowed
	}
	if carriesBelow(root, agentValuesMarker) {
		return nil, errors.New("x-agent-values is read on a top-level body member only")
	}
	return narrowed, nil
}

// agentValues reads one member's marker against its own enum.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func agentValues(name string, fields map[string]any) ([]string, error) {
	listed, isList := fields[agentValuesMarker].([]any)
	enum, _ := fields["enum"].([]any)
	if !isList || len(listed) == 0 || len(enum) == 0 {
		return nil, fmt.Errorf("body member %s: x-agent-values lists values of the member's enum", name)
	}
	allowed := make([]string, 0, len(listed))
	for _, value := range listed {
		word, isWord := value.(string)
		if !isWord || !slices.Contains(enum, any(word)) {
			return nil, fmt.Errorf("body member %s: x-agent-values names %v, which its enum %v does not hold", name, value, enum)
		}
		allowed = append(allowed, word)
	}
	return allowed, nil
}

// carriesBelow reports marker left anywhere in node.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func carriesBelow(node any, marker string) bool {
	switch v := node.(type) {
	case map[string]any:
		if v[marker] != nil {
			return true
		}
		for _, member := range v {
			if carriesBelow(member, marker) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, func(member any) bool { return carriesBelow(member, marker) })
	}
	return false
}

//craft:ignore naked-any an enum is written back as decoded JSON, whatever its values
func toAny(words []string) []any {
	out := make([]any, len(words))
	for i, word := range words {
		out[i] = word
	}
	return out
}
