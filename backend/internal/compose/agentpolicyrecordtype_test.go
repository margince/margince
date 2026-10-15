// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"testing"
)

// A route's record_type is one its tool accepts, read from the record_type enum
// of the tool's published schema. A tool with no record_type property is
// exempt: there the route's record_type is only a label. served_by: rest_only
// declares the exception. A marker the tool no longer needs fails too, so the
// route goes back to naming the tool that serves it.
func TestEveryPolicyRecordTypeIsOneItsToolCanSend(t *testing.T) {
	accepts := map[string][]string{}
	for _, spec := range servedSurface(t).Specs() {
		accepts[spec.Name] = recordTypeEnum(t, spec.Name, spec.InputSchema)
	}
	for route, policy := range agentPolicies {
		restOnly := policy.ServedBy == servedByRestOnly
		enum, registered := accepts[policy.Tool]
		if policy.Tool == "" || policy.RecordType == "" || !registered || enum == nil {
			if restOnly {
				t.Errorf("%s is marked served_by: rest_only, but it names no record type a tool "+
					"could refuse. Remove the marker.", route)
			}
			continue // TestEveryDeclaredToolVerbIsRegistered owns an unregistered verb.
		}
		accepted := slices.Contains(enum, string(policy.RecordType))
		switch {
		case restOnly && accepted:
			t.Errorf("%s is marked served_by: rest_only, but tool %q now accepts record_type %q. "+
				"Remove the marker so the route names the tool that serves it.",
				route, policy.Tool, policy.RecordType)
		case !restOnly && !accepted:
			t.Errorf("%s declares tool %q with record_type %q, which that tool's schema does not "+
				"accept — the route reads as agent-reachable on MCP and answers nothing there. Serve "+
				"the type, name the tool that does, mark it served_by: rest_only, or declare it human-only.",
				route, policy.Tool, policy.RecordType)
		}
	}
}

// recordTypeEnum reads the record_type values a tool's input schema accepts. It
// answers nil for a tool with no record_type property. A property with no enum
// fails the test rather than reading as one that accepts nothing.
func recordTypeEnum(t *testing.T, tool string, schema json.RawMessage) []string {
	t.Helper()
	var parsed struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("tool %q publishes an input schema that is not JSON: %v", tool, err)
	}
	property, declared := parsed.Properties["record_type"]
	if !declared {
		return nil
	}
	if len(property.Enum) == 0 {
		t.Fatalf("tool %q declares record_type with no enum, so no route's record_type can be held to it", tool)
	}
	return property.Enum
}
