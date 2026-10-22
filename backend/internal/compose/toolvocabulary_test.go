// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A tool and its route are two transports onto one behaviour, and a vocabulary
// is part of that behaviour: a phase or a link target the REST body accepts and
// the tool refuses is the same divergence as a missing tool, arriving one
// argument at a time.
//
// Derived in both directions rather than listed. Every registered tool's
// advertised schema is walked for closed vocabularies, and every one is
// compared against the operation the policy table says backs that verb. A new
// tool with an enum is enrolled the day it is written, and a tool argument with
// no REST twin is skipped by the DERIVATION (no backing operation declares a
// property of that name), never by being forgotten.

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestEveryToolEnumMatchesTheContractItMirrors(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}
	bodies := requestBodiesByOperation(doc)
	registry := NewRegistry(nil, SendPath{})

	compared := 0
	for _, spec := range registry.Specs() {
		for _, advertised := range advertisedEnums(t, spec.Name, spec.InputSchema) {
			for _, op := range backingOperations(spec.Name) {
				declared, isEnum := contractEnum(bodies[op], advertised.name)
				if !isEnum {
					continue // the operation takes no such argument, or takes it open
				}
				compared++
				if !slices.Equal(advertised.values, declared) {
					t.Errorf("%s advertises %v for %s while %s's request body declares %v — a value the "+
						"REST call accepts and the tool refuses (or the reverse) is the two transports "+
						"disagreeing about one behaviour", spec.Name, advertised.values, advertised.name, op, declared)
				}
			}
		}
	}
	if compared == 0 {
		t.Fatal("no tool argument was compared against a contract enum — this gate asserted nothing")
	}
}

// backingOperations answers the operationIds the generated policy table says a
// verb backs; a tool composing several operations (or none) yields each of them.
func backingOperations(tool string) []string {
	var ops []string
	for _, pol := range agentPolicies {
		if pol.Access == accessTool && pol.Tool == tool && !slices.Contains(ops, pol.Op) {
			ops = append(ops, pol.Op)
		}
	}
	slices.Sort(ops)
	return ops
}

// requestBodiesByOperation indexes each operation's request-body schema,
// resolved through its $refs so an inline body and a component read alike.
func requestBodiesByOperation(doc *openapi3.T) map[string]*openapi3.Schema {
	bodies := map[string]*openapi3.Schema{}
	for _, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if op.RequestBody == nil || op.RequestBody.Value == nil {
				continue
			}
			for _, media := range op.RequestBody.Value.Content {
				if media.Schema != nil && media.Schema.Value != nil {
					bodies[op.OperationID] = media.Schema.Value
				}
			}
		}
	}
	return bodies
}

// contractEnum reads one property's declared vocabulary, reporting whether the
// operation declares that property as a closed set at all. Each member is
// spelled with its JSON type, so a string "3" and a number 3 differ.
func contractEnum(body *openapi3.Schema, member string) ([]string, bool) {
	if body == nil {
		return nil, false
	}
	prop, declared := body.Properties[member]
	if !declared || prop.Value == nil || len(prop.Value.Enum) == 0 {
		return nil, false
	}
	// A nullable enum carries null as its "unset" member (logActivity's
	// direction does). A tool expresses that by omitting the argument, so null
	// is not part of the vocabulary being compared.
	return enumMembers(slices.DeleteFunc(slices.Clone(prop.Value.Enum), func(v any) bool { return v == nil })), true
}

// toolEnum is one closed vocabulary a tool's schema declares, at any depth.
type toolEnum struct {
	name   string
	values []string
}

// advertisedEnums answers every closed vocabulary a tool's own schema declares,
// nested ones included: an offer line's billing interval sits under `line`.
func advertisedEnums(t *testing.T, tool string, inputSchema json.RawMessage) []toolEnum {
	t.Helper()
	var enums []toolEnum
	if err := collectEnums(inputSchema, &enums); err != nil {
		t.Fatalf("%s: input schema is not readable: %v", tool, err)
	}
	return enums
}

func collectEnums(schema json.RawMessage, into *[]toolEnum) error {
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		return err
	}
	for name, raw := range doc.Properties {
		var prop struct {
			Enum []any `json:"enum"`
		}
		if err := json.Unmarshal(raw, &prop); err != nil {
			return err
		}
		if len(prop.Enum) > 0 {
			*into = append(*into, toolEnum{name: name, values: enumMembers(prop.Enum)})
		}
		if err := collectEnums(raw, into); err != nil {
			return err
		}
	}
	return nil
}

// enumMembers spells each member with its JSON type, sorted.
func enumMembers(members []any) []string {
	out := make([]string, 0, len(members))
	for _, v := range members {
		switch v.(type) {
		case string:
			out = append(out, fmt.Sprintf("%q", v))
		default:
			out = append(out, fmt.Sprintf("%v", v))
		}
	}
	slices.Sort(out)
	return out
}
