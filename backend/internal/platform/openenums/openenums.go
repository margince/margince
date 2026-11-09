// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package openenums holds the contract's enum rule: an enum a response can
// carry is open, and an enum only a request carries stays closed.
//
// A client that meets an enum value it does not know must keep the rest of the
// response, so a response enum is marked `x-extensible-enum: true`. A request
// enum stays closed, because the server refuses a value it cannot honour.
//
// Both sides are derived from the contract's operations. An enum is
// response-side when a response body or header reaches it through $ref, and
// request-side when a request body or parameter does. An enum both reach is
// response-side; the server still validates the request value.
package openenums

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Marker is the key an open enum carries, with the value true.
const Marker = "x-extensible-enum"

// Enum is one enum schema the contract's operations reach.
type Enum struct {
	Line     int
	Response bool
	Request  bool
	Open     bool

	node *yaml.Node
}

// Classify reads a contract and answers every enum its operations reach, in
// source order.
func Classify(raw []byte) ([]Enum, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("openenums: parsing the contract: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, errors.New("openenums: the contract is empty")
	}
	w := &walker{root: doc.Content[0], found: map[*yaml.Node]*Enum{}, seen: map[string]bool{}}
	paths := mapValue(w.root, "paths")
	if paths == nil {
		return nil, errors.New("openenums: the contract has no paths")
	}
	for i := 1; i < len(paths.Content); i += 2 {
		w.pathItem(w.resolve(paths.Content[i]))
	}
	out := make([]Enum, 0, len(w.found))
	for _, e := range w.found {
		marker := mapValue(e.node, Marker)
		e.Open = marker != nil && marker.Value == "true"
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out, nil
}

// Violations names every enum that breaks the rule, by line.
func Violations(path string, enums []Enum) []string {
	var out []string
	for _, e := range enums {
		switch {
		case e.Response && !e.Open:
			out = append(out, fmt.Sprintf("%s:%d: a response can carry this enum and it is closed; mark it `%s: true`",
				path, e.Line, Marker))
		case !e.Response && mapValue(e.node, Marker) != nil:
			out = append(out, fmt.Sprintf("%s:%d: only a request carries this enum, so it stays closed; remove `%s`",
				path, e.Line, Marker))
		}
	}
	return out
}

// walker follows the contract from its operations to the enums they reach.
type walker struct {
	root  *yaml.Node
	found map[*yaml.Node]*Enum
	seen  map[string]bool
}

var operationVerbs = []string{"get", "put", "post", "delete", "patch", "head", "options", "trace"}

func (w *walker) pathItem(item *yaml.Node) {
	w.parameters(mapValue(item, "parameters"))
	for _, verb := range operationVerbs {
		op := mapValue(item, verb)
		if op == nil {
			continue
		}
		w.parameters(mapValue(op, "parameters"))
		if body := w.resolve(mapValue(op, "requestBody")); body != nil {
			w.content(mapValue(body, "content"), false)
		}
		responses := mapValue(op, "responses")
		for i := 1; responses != nil && i < len(responses.Content); i += 2 {
			response := w.resolve(responses.Content[i])
			w.content(mapValue(response, "content"), true)
			headers := mapValue(response, "headers")
			for j := 1; headers != nil && j < len(headers.Content); j += 2 {
				w.schema(mapValue(w.resolve(headers.Content[j]), "schema"), true)
			}
		}
	}
}

func (w *walker) parameters(params *yaml.Node) {
	if params == nil {
		return
	}
	for _, param := range params.Content {
		w.schema(mapValue(w.resolve(param), "schema"), false)
	}
}

func (w *walker) content(content *yaml.Node, response bool) {
	for i := 1; content != nil && i < len(content.Content); i += 2 {
		w.schema(mapValue(content.Content[i], "schema"), response)
	}
}

// The keys whose value is one schema, a list of schemas, or a map of them.
var (
	schemaChildren = []string{"items", "additionalProperties", "not", "contains", "if", "then", "else",
		"unevaluatedProperties", "unevaluatedItems", "propertyNames"}
	schemaLists = []string{"allOf", "oneOf", "anyOf", "prefixItems"}
	schemaMaps  = []string{"properties", "patternProperties", "dependentSchemas", "$defs"}
)

func (w *walker) schema(node *yaml.Node, response bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	if ref := mapValue(node, "$ref"); ref != nil {
		key := fmt.Sprintf("%s|%v", ref.Value, response)
		if !w.seen[key] {
			w.seen[key] = true
			w.schema(w.resolve(node), response)
		}
		return
	}
	if values := mapValue(node, "enum"); values != nil {
		e := w.found[node]
		if e == nil {
			e = &Enum{Line: values.Line, node: node}
			w.found[node] = e
		}
		e.Response = e.Response || response
		e.Request = e.Request || !response
	}
	for _, key := range schemaChildren {
		w.schema(mapValue(node, key), response)
	}
	for _, key := range schemaLists {
		for _, item := range listItems(mapValue(node, key)) {
			w.schema(item, response)
		}
	}
	for _, key := range schemaMaps {
		if m := mapValue(node, key); m != nil {
			for i := 1; i < len(m.Content); i += 2 {
				w.schema(m.Content[i], response)
			}
		}
	}
}

// resolve follows a local $ref, repeatedly, to the node it names.
func (w *walker) resolve(node *yaml.Node) *yaml.Node {
	for node != nil {
		ref := mapValue(node, "$ref")
		if ref == nil {
			return node
		}
		target := w.root
		for _, part := range strings.Split(strings.TrimPrefix(ref.Value, "#/"), "/") {
			target = mapValue(target, strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~"))
		}
		node = target
	}
	return nil
}

func listItems(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

// mapValue is the value under key in a mapping node, or nil.
func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
