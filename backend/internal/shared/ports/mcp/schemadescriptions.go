// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcp

// dataKeys hold values a model reads as data, not as schema. A "description"
// inside one is a member of that data, so no walk below reads or rewrites it.
var dataKeys = map[string]bool{"enum": true, "const": true, "default": true, "examples": true}

// namedSchemas are the keywords whose children are schemas keyed by a name the
// schema's author chose. A key there is a name, whatever word it spells.
var namedSchemas = map[string]bool{"properties": true, "patternProperties": true, "$defs": true, "definitions": true}

// RewriteSchemaDescriptions replaces each schema "description" string under a
// decoded JSON schema with fn's answer, deleting the key when the answer is
// empty. It reports whether any changed. The registry reads the names a schema
// mentions through it. The runner drops the sentences naming a tool the run is
// not offered. Both see the same strings.
//
//craft:ignore naked-any a decoded JSON schema is any by construction; the walk reads its maps, slices and strings
func RewriteSchemaDescriptions(node any, fn func(string) string) bool {
	return rewriteDescriptions(node, fn, false)
}

//craft:ignore naked-any a decoded JSON schema is any by construction; the walk reads its maps, slices and strings
func rewriteDescriptions(node any, fn func(string) string, names bool) bool {
	changed := false
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if rewriteMember(v, key, child, fn, names) {
				changed = true
			}
		}
	case []any:
		for _, child := range v {
			if rewriteDescriptions(child, fn, false) {
				changed = true
			}
		}
	}
	return changed
}

// rewriteMember handles one member of a schema object. A name is walked as a
// schema, a data value is left alone, and a description is rewritten.
//
//craft:ignore naked-any a decoded JSON schema is any by construction; the walk reads its maps, slices and strings
func rewriteMember(object map[string]any, key string, child any, fn func(string) string, names bool) bool {
	if names {
		return rewriteDescriptions(child, fn, false)
	}
	if dataKeys[key] {
		return false
	}
	text, isText := child.(string)
	if !isText || key != "description" {
		return rewriteDescriptions(child, fn, namedSchemas[key])
	}
	kept := fn(text)
	if kept == text {
		return false
	}
	if kept == "" {
		delete(object, key)
		return true
	}
	object[key] = kept
	return true
}
