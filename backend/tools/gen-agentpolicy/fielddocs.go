// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "strings"

// refWalk is one schema's $ref resolution: the schemas it reads, the refs open on
// the current path, and whether each body field keeps its description.
type refWalk struct {
	schemas   map[string]any
	open      map[string]bool
	fieldDocs bool
}

// resolveProperties resolves a `properties` map, whose keys are field names and
// so are never prose: a field may be called description.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (w *refWalk) resolveProperties(named map[string]any, depth int) (map[string]any, error) {
	out := make(map[string]any, len(named))
	for field, member := range named {
		resolved, err := w.resolve(member, depth)
		if err != nil {
			return nil, err
		}
		if doc := w.fieldDoc(member); doc != "" {
			if fields, isMap := resolved.(map[string]any); isMap {
				fields["description"] = doc
			}
		}
		out[field] = resolved
	}
	return out, nil
}

// fieldDoc is a body field's description: its own, or else the one of the
// schema its $ref names. Empty outside a body, where the words stay out.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func (w *refWalk) fieldDoc(member any) string {
	fields, isMap := member.(map[string]any)
	if !w.fieldDocs || !isMap {
		return ""
	}
	if doc, ok := fields["description"].(string); ok {
		return oneLine(doc)
	}
	ref, _ := fields["$ref"].(string)
	target, _ := w.schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
	doc, _ := target["description"].(string)
	return oneLine(doc)
}

// oneLine is a contract description as a body field carries it.
func oneLine(doc string) string {
	return strings.Join(strings.Fields(doc), " ")
}
