// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package schema builds JSON Schema values for constraining structured model
// output (the model.Request.ResponseSchema field). Callers compose
// Record/Object/Array/String/… and render with Must instead of hand-writing a
// JSON string, so every structured-output schema is compile-checked, always
// valid JSON, and built one way across the codebase.
//
// WHERE THE LINE SITS. The vocabulary is the strict structured-output profile
// and nothing wider: closed objects, every property required, `type` (string,
// number, integer, array, object, null), `enum`, `items`, `anyOf` and the
// annotations. That is the one shape every provider adapter can send without
// the endpoint refusing it — OpenAI's strict mode takes it
// (modules/ai/schemastrict.go is the allowlist;
// TestEverySchemaTheBuilderComposesIsStrictEligible composes every Node
// constructor below against it and fails on one it does not know), and
// Gemini, Ollama and vLLM accept it.
//
// Value BOUNDS are outside it on purpose — no maxLength, maxItems, minimum or
// uniqueItems. Anthropic refuses a schema carrying them outright, strict mode
// refuses most of them, and Mistral refused `uniqueItems` with a 400 that
// failed a call which had been answering. A bound belongs in the caller's own
// validation of the result, read from the same named constant the prompt
// states, never in the schema, which only pins the SHAPE at generation.
package schema

import (
	"encoding/json"
	"sort"
	"strings"
)

// Node is one JSON Schema node. The zero value is not useful; build nodes with
// the constructors below.
type Node struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	// additionalProperties is JSON Schema's spec keyword — camelCase by the
	// spec, not a style choice; snake_case would be an invalid keyword.
	AdditionalProperties *bool           `json:"additionalProperties,omitempty"` //nolint:tagliatelle // JSON Schema spec keyword, must be camelCase
	Properties           map[string]Node `json:"properties,omitempty"`
	Items                *Node           `json:"items,omitempty"`
	Required             []string        `json:"required,omitempty"`
	// anyOf is the spec keyword, and here it holds exactly one shape: a value
	// or null (see Optional).
	AnyOf []Node `json:"anyOf,omitempty"` //nolint:tagliatelle // JSON Schema spec keyword, must be camelCase

	// order is the declared property order of a Record. Empty for an Object,
	// whose properties render sorted by name as encoding/json renders a map.
	order []string
}

// Describe attaches the JSON Schema `description` annotation — guidance the
// model reads for what a field means. It returns a copy so it chains onto any
// node: schema.String().Describe("the customer's full legal name"). All
// supported providers (Ollama, vLLM, Anthropic) accept the standard keyword.
func (n Node) Describe(desc string) Node {
	n.Description = desc
	return n
}

// The Node.Type values this package's vocabulary spans — shared with
// validate.go's ValidateJSON so the builder and the validator never drift
// on the type-name strings they switch over.
const (
	typeObject  = "object"
	typeArray   = "array"
	typeString  = "string"
	typeNumber  = "number"
	typeInteger = "integer"
	typeNull    = "null"
)

// String is a JSON string leaf.
func String() Node { return Node{Type: typeString} }

// Number is a JSON number leaf (integer or float).
func Number() Node { return Node{Type: typeNumber} }

// Integer is a whole-number leaf. Distinct from Number because a line number,
// a count or an index decodes into an int, and a model offered `number` is
// free to answer 3.5 where the reader can only refuse it.
func Integer() Node { return Node{Type: typeInteger} }

// Array is a list whose every item matches items.
func Array(items Node) Node { return Node{Type: typeArray, Items: &items} }

// Enum is a string leaf constrained to one of values (JSON Schema `enum`).
// All supported providers constrain generation to the given set.
//
// No values renders an open string rather than `"enum":[]`, which no value
// satisfies: a per-call enum built from an empty list (no passages to cite)
// still describes a field, and the caller's validator refuses what it cannot
// accept. A site whose call is meaningless with an empty list guards it before
// the request, as stage_evidence_extract does with no criteria.
func Enum(values ...string) Node { return Node{Type: typeString, Enum: values} }

// Optional is n or null — the ONE way this vocabulary says a value may be
// absent. Leaving a property out of `required` says it too, but the strict
// profile refuses that object whole, so the field stays required and its
// absence is spelled as null. A reader decoding into a Go string or slice gets
// the zero value for null.
func Optional(n Node) Node { return Node{AnyOf: []Node{n, {Type: typeNull}}} }

// Object is a closed object: props are its properties and required names the
// ones that must be present. Properties render sorted by name; use Record when
// the order the model writes them in matters, or when every property is
// required anyway.
func Object(props map[string]Node, required ...string) Node {
	closed := false
	return Node{Type: typeObject, AdditionalProperties: &closed, Properties: props, Required: required}
}

// Property is one named member of a Record.
type Property struct {
	name string
	node Node
}

// Field names one member of a Record.
func Field(name string, n Node) Property { return Property{name: name, node: n} }

// Record is a closed object whose every field is required and whose
// properties render in the order given — the order a constrained decoder
// writes them in. Order is not cosmetic under constrained decoding: a model
// that writes its verdict before its evidence has decided before reading, and
// the agent loop measured a reordered schema at a quarter of its score.
//
// A repeated name panics: it is a programmer error, and a schema rendering one
// key twice is not a schema any endpoint reads the same way.
func Record(fields ...Property) Node {
	props := make(map[string]Node, len(fields))
	order := make([]string, 0, len(fields))
	for _, f := range fields {
		if _, repeated := props[f.name]; repeated {
			panic("schema: record declares field " + f.name + " twice")
		}
		props[f.name] = f.node
		order = append(order, f.name)
	}
	n := Object(props, order...)
	n.order = order
	return n
}

// wireNode is Node as it goes on the wire: the same keywords in the same
// order, with properties written in the node's declared order.
//
// A second struct rather than Node embedded, because embedding cannot replace
// one field's encoding in place — the ordered `properties` would move to the
// end and change the bytes every existing schema renders to, which a prompt
// version is stamped from. The two field lists are one list spelled twice;
// TestTheWireNodeCarriesEveryNodeKeyword holds them to the same tags in the
// same order, so a keyword added to Node cannot silently drop off the wire.
type wireNode struct {
	Type                 string          `json:"type,omitempty"`
	Description          string          `json:"description,omitempty"`
	Enum                 []string        `json:"enum,omitempty"`
	AdditionalProperties *bool           `json:"additionalProperties,omitempty"` //nolint:tagliatelle // JSON Schema spec keyword, must be camelCase
	Properties           *orderedMembers `json:"properties,omitempty"`
	Items                *Node           `json:"items,omitempty"`
	Required             []string        `json:"required,omitempty"`
	AnyOf                []Node          `json:"anyOf,omitempty"` //nolint:tagliatelle // JSON Schema spec keyword, must be camelCase
}

// MarshalJSON renders the node. An Object's properties come out sorted by
// name — byte-identical to encoding/json's rendering of the map — and a
// Record's in the order it declared them.
func (n Node) MarshalJSON() ([]byte, error) {
	wire := wireNode{
		Type: n.Type, Description: n.Description, Enum: n.Enum,
		AdditionalProperties: n.AdditionalProperties, Items: n.Items,
		Required: n.Required, AnyOf: n.AnyOf,
	}
	if len(n.Properties) > 0 {
		wire.Properties = &orderedMembers{names: n.propertyOrder(), byName: n.Properties}
	}
	return json.Marshal(wire)
}

// propertyOrder is the declared order when there is one, and name order
// otherwise.
func (n Node) propertyOrder() []string {
	if len(n.order) > 0 {
		return n.order
	}
	names := make([]string, 0, len(n.Properties))
	for name := range n.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// orderedMembers is a `properties` map written in a chosen key order, which a
// Go map cannot hold.
type orderedMembers struct {
	names  []string
	byName map[string]Node
}

func (m orderedMembers) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, name := range m.names {
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(m.byName[name])
		if err != nil {
			return nil, err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// Must renders a node to the wire bytes for ResponseSchema. It panics only on
// a programmer error — a Node cannot fail to marshal — so it is safe in a
// package-level var initializer.
func Must(n Node) json.RawMessage {
	raw, err := json.Marshal(n)
	if err != nil {
		panic("schema: rendering node: " + err.Error())
	}
	return raw
}

// Names is values as the plain strings Enum takes — for a closed vocabulary
// declared as a named string type, so an enum is derived from the type's own
// constants rather than re-spelled beside them.
func Names[T ~string](values ...T) []string {
	names := make([]string, 0, len(values))
	for _, v := range values {
		names = append(names, string(v))
	}
	return names
}
