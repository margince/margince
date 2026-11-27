// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcp

// dataKeys hold values a model reads as data, not as schema. A "description"
// inside one is a member of that data, so no walk below reads or rewrites it.
var dataKeys = map[string]bool{"enum": true, "const": true, "default": true, "examples": true}

// RewriteSchemaDescriptions replaces each schema "description" string under a
// decoded JSON schema with fn's answer, deleting the key when the answer is
// empty. It reports whether any changed. The registry reads the names a schema
// mentions through it. The runner drops the sentences naming a tool the run is
// not offered. Both see the same strings.
//
//craft:ignore naked-any a decoded JSON schema is any by construction; the walk reads its maps, slices and strings
func RewriteSchemaDescriptions(node any, fn func(string) string) bool {
	changed := false
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if dataKeys[key] {
				continue
			}
			if text, ok := child.(string); ok && key == "description" {
				if kept := fn(text); kept != text {
					changed = true
					if kept == "" {
						delete(v, key)
					} else {
						v[key] = kept
					}
				}
				continue
			}
			if RewriteSchemaDescriptions(child, fn) {
				changed = true
			}
		}
	case []any:
		for _, child := range v {
			if RewriteSchemaDescriptions(child, fn) {
				changed = true
			}
		}
	}
	return changed
}
