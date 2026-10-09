// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package oasnode reads an OpenAPI document as a yaml.Node tree. The generators
// that cut part of the contract out share it, so they agree on what an
// operation is and what a $ref points at.
package oasnode

import "gopkg.in/yaml.v3"

// HTTPMethods maps every operation key a path item may carry onto the method
// a router registers. A generator that missed one would leave that operation
// out of whatever it derives.
var HTTPMethods = map[string]string{
	"get": "GET", "head": "HEAD", "options": "OPTIONS", "trace": "TRACE",
	"post": "POST", "put": "PUT", "patch": "PATCH", "delete": "DELETE",
}

// Root unwraps a parsed document to its top-level mapping.
func Root(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		return doc.Content[0]
	}
	return doc
}

// Lookup finds a mapping's value node by key.
func Lookup(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}

// Scalar returns the scalar value stored under key, or "".
func Scalar(node *yaml.Node, key string) string {
	value, ok := Lookup(node, key)
	if !ok || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

// Refs collects every $ref value under a node, in document order. The values
// of a discriminator's mapping are refs too, which a reader follows to a schema.
func Refs(node *yaml.Node) []string {
	var refs []string
	if node.Kind == yaml.MappingNode {
		if ref := Scalar(node, "$ref"); ref != "" {
			refs = append(refs, ref)
		}
		discriminator, _ := Lookup(node, "discriminator")
		if mapping, ok := Lookup(discriminator, "mapping"); ok {
			for i := 1; i < len(mapping.Content); i += 2 {
				refs = append(refs, mapping.Content[i].Value)
			}
		}
	}
	for _, child := range node.Content {
		refs = append(refs, Refs(child)...)
	}
	return refs
}
